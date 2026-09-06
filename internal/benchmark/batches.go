package benchmark

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// benchmarkBatchSize bounds one Mihomo instance so a malformed node can be
// isolated by bisection without dragging the whole candidate set down, and so
// no single instance is saturated by probes for every candidate at once.
const benchmarkBatchSize = 100
const probeWorkers = 24

// benchmarkStartRetries bounds retries for transient bring-up failures such as
// a port snatched between allocation and Mihomo binding.
const benchmarkStartRetries = 3

// Benchmark probes candidates in bounded batches. If Mihomo's config check
// rejects a batch, the batch is recursively bisected so only malformed nodes
// are dropped and the healthy remainder is still measured. droppedNames lists
// the nodes removed by bisection. An engine-level failure aborts the remaining
// batches and is reported via err alongside any metrics already collected; a
// context deadline is also surfaced as an error so a timed-out probe is never
// mistaken for a dead node pool.
func Benchmark(ctx context.Context, core string, proxies []map[string]any, candidates []Candidate, cfg Config) ([]Metric, []string, error) {
	if len(proxies) != len(candidates) {
		return nil, nil, fmt.Errorf("proxy and candidate counts do not match")
	}
	for index, proxy := range proxies {
		name, ok := proxy["name"].(string)
		if !ok || name == "" || candidates[index].Name != name {
			return nil, nil, fmt.Errorf("proxy and candidate names do not match at index %d", index)
		}
	}
	var metrics []Metric
	var dropped []string
	for offset := 0; offset < len(proxies); offset += benchmarkBatchSize {
		end := offset + benchmarkBatchSize
		if end > len(proxies) {
			end = len(proxies)
		}
		batchMetrics, batchDropped, err := runBatch(ctx, core, proxies[offset:end], candidates[offset:end], cfg)
		metrics = append(metrics, batchMetrics...)
		dropped = append(dropped, batchDropped...)
		if err != nil {
			return metrics, dropped, err
		}
		// A deadline that expires inside a batch leaves that batch's metrics as
		// all-ineligible zeroes; surface the cancellation as an error so the
		// caller never mistakes a timed-out probe for a dead node pool.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return metrics, dropped, ctxErr
		}
	}
	return metrics, dropped, nil
}

// runBatch brings up Mihomo for one batch, probes it, and bisects the batch
// when the config is rejected so only the malformed node is discarded.
func runBatch(ctx context.Context, core string, proxies []map[string]any, candidates []Candidate, cfg Config) ([]Metric, []string, error) {
	// A dead context makes every Mihomo -t check fail with a wrapped config
	// error; without this guard the timeout would silently convert every
	// remaining node into a bisection drop instead of surfacing as an error.
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var lastErr error
	for attempt := 0; attempt < benchmarkStartRetries; attempt++ {
		config, err := BuildBenchmarkConfig(proxies)
		if err != nil {
			lastErr = err
			break
		}
		mihomo, err := NewMihomo(ctx, core, config, "")
		if err == nil {
			metrics := Probe(ctx, mihomo.API, candidates, cfg)
			_ = mihomo.Close()
			return metrics, nil, nil
		}
		var rejected *ConfigRejectedError
		if errors.As(err, &rejected) {
			if len(proxies) <= 1 {
				// A single node that cannot pass Mihomo's config check is the
				// only possible cause of its own rejection, whether or not the
				// output names it; drop it instead of aborting the batch.
				return nil, []string{candidates[0].Name}, nil
			}
			if mentionsCandidate(rejected.Output, candidates) {
				// Only a rejection that names one of the candidates can be
				// attributed to a node; bisect to isolate the malformed nodes.
				return bisect(ctx, core, proxies, candidates, cfg)
			}
			// The rejection names no candidate, so bisection cannot attribute
			// it. Isolate every node on its own: a malformed node whose error
			// text does not mention it is still dropped, and the healthy
			// remainder is measured instead of aborting the whole batch.
			return isolate(ctx, core, proxies, candidates, cfg)
		}
		lastErr = err
		if !isPortConflict(err) {
			// Engine-level failure: do not disguise it as a bad node.
			return nil, nil, err
		}
	}
	if len(proxies) == 0 {
		return nil, nil, nil
	}
	return nil, nil, lastErr
}

// bisect splits a rejected batch in halves until each node is isolated; single
// nodes that still fail are dropped.
func bisect(ctx context.Context, core string, proxies []map[string]any, candidates []Candidate, cfg Config) ([]Metric, []string, error) {
	if len(proxies) <= 1 {
		return nil, []string{candidates[0].Name}, nil
	}
	mid := len(proxies) / 2
	leftMetrics, leftDropped, err := runBatch(ctx, core, proxies[:mid], candidates[:mid], cfg)
	if err != nil {
		return leftMetrics, leftDropped, err
	}
	rightMetrics, rightDropped, err := runBatch(ctx, core, proxies[mid:], candidates[mid:], cfg)
	return append(leftMetrics, rightMetrics...), append(leftDropped, rightDropped...), err
}

// isolate probes every candidate of a rejected batch on its own, dropping the
// singletons that still fail Mihomo's config check. A rejection that survives
// even in isolation cannot be attributed to a node and is reported as an
// engine-level error alongside the metrics already collected.
func isolate(ctx context.Context, core string, proxies []map[string]any, candidates []Candidate, cfg Config) ([]Metric, []string, error) {
	var metrics []Metric
	var dropped []string
	for index := range proxies {
		batchMetrics, batchDropped, err := runBatch(ctx, core, proxies[index:index+1], candidates[index:index+1], cfg)
		metrics = append(metrics, batchMetrics...)
		dropped = append(dropped, batchDropped...)
		if err != nil {
			return metrics, dropped, err
		}
	}
	return metrics, dropped, nil
}

// mentionsCandidate reports whether the rejection output names one of the
// candidates. A name only counts when it appears at a word boundary: a bare
// substring match would misattribute errors containing ordinary English words
// (e.g. "us" inside "status" or "because") to short node names. The bytes
// adjacent to the match must not be ASCII word characters; non-ASCII names
// (CJK, emoji) cannot collide with ASCII error text and match wherever they
// appear.
func mentionsCandidate(output string, candidates []Candidate) bool {
	for _, candidate := range candidates {
		name := candidate.Name
		if name == "" {
			continue
		}
		for offset := 0; offset+len(name) <= len(output); {
			index := strings.Index(output[offset:], name)
			if index < 0 {
				break
			}
			index += offset
			startOK := index == 0 || !isWordByte(output[index-1])
			end := index + len(name)
			endOK := end == len(output) || !isWordByte(output[end])
			if startOK && endOK {
				return true
			}
			offset = index + 1
		}
	}
	return false
}

// isWordByte reports whether b is an ASCII word character (letter, digit, or
// underscore), which delimits tokens in Mihomo's error text the way \b does
// for pure ASCII output.
func isWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func isPortConflict(err error) bool {
	if err == nil {
		return false
	}
	detail := strings.ToLower(err.Error())
	return strings.Contains(detail, "address already in use") || strings.Contains(detail, "only one usage of each socket address")
}

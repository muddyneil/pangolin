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

// benchmarkStartRetries bounds retries for transient bring-up failures such as
// a port snatched between allocation and Mihomo binding.
const benchmarkStartRetries = 3

// Benchmark probes candidates in bounded batches. If Mihomo's config check
// rejects a batch, the batch is recursively bisected so only malformed nodes
// are dropped and the healthy remainder is still measured. droppedNames lists
// the nodes removed by bisection. An engine-level failure aborts the remaining
// batches and is reported via err alongside any metrics already collected.
func Benchmark(ctx context.Context, core string, proxies []map[string]any, candidates []Candidate, cfg Config) ([]Metric, []string, error) {
	if len(proxies) != len(candidates) {
		return nil, nil, fmt.Errorf("proxies 与 candidates 数量不一致")
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
			// At least one candidate makes the config invalid: split until it
			// is isolated and discarded, keeping the healthy remainder.
			return bisect(ctx, core, proxies, candidates, cfg)
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

func isPortConflict(err error) bool {
	if err == nil {
		return false
	}
	detail := strings.ToLower(err.Error())
	return strings.Contains(detail, "address already in use") || strings.Contains(detail, "only one usage of each socket address")
}

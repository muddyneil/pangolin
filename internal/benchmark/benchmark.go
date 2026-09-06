package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Candidate struct{ Name string }
type BenchmarkConfig struct {
	Proxies       []map[string]any `json:"proxies"`
	Groups        []map[string]any `json:"proxy-groups"`
	UnifiedDelay  bool             `json:"unified-delay"`
	TCPConcurrent bool             `json:"tcp-concurrent"`
	IPv6          bool             `json:"ipv6"`
}

// BuildBenchmarkConfig builds a minimal Mihomo config that lists every
// candidate proxy in a select group. Ports and controller settings are not
// emitted here; withPorts in the mihomo package injects them. unified-delay,
// tcp-concurrent and ipv6 mirror the reference generator's benchmark config so
// delay tests measure the same thing on the same nodes.
func BuildBenchmarkConfig(proxies []map[string]any) ([]byte, error) {
	group := map[string]any{"name": "BENCHMARK", "type": "select", "proxies": []string{"DIRECT"}}
	for _, proxy := range proxies {
		if name, ok := proxy["name"].(string); ok && name != "" {
			group["proxies"] = append(group["proxies"].([]string), name)
		}
	}
	return json.Marshal(BenchmarkConfig{Proxies: proxies, Groups: []map[string]any{group}, UnifiedDelay: true, TCPConcurrent: true, IPv6: true})
}

type Config struct {
	ProbeTimes   int
	ProbeTimeout time.Duration
	MaxLatency   time.Duration
	MaxJitter    time.Duration
	URLs         []string
}
type Metric struct {
	Name        string
	LatencyMs   int
	PassCount   int
	JitterMs    int
	HealthScore int
	// Attempts and Successes count delay probes at request level; they feed
	// the availability gate in Eligible so a transient burst at probe time
	// does not discard a healthy node.
	Attempts  int
	Successes int
}

// ProbeTimeout is the per-URL delay budget passed to Mihomo's /delay API.
// It must stay well below the controller client timeout so Mihomo's own budget,
// not the client deadline, decides a failed probe.
func DefaultConfig() Config {
	return Config{ProbeTimes: 3, ProbeTimeout: 2 * time.Second, MaxLatency: 800 * time.Millisecond, MaxJitter: 300 * time.Millisecond, URLs: []string{"https://cp.cloudflare.com/generate_204", "http://www.gstatic.com/generate_204", "https://connectivitycheck.android.com/generate_204"}}
}

// EstimatedTimeout accounts for the actual benchmark batch size and worker
// waves. The floor covers startup and config-validation overhead.
func EstimatedTimeout(candidateCount int, cfg Config) time.Duration {
	if candidateCount <= 0 {
		return 15 * time.Minute
	}
	probeTimeout := cfg.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = 5 * time.Second
	}
	probeTimes := cfg.ProbeTimes
	if probeTimes < 3 {
		probeTimes = 3
	}
	batches := (candidateCount + benchmarkBatchSize - 1) / benchmarkBatchSize
	waves := (benchmarkBatchSize + probeWorkers - 1) / probeWorkers
	budget := time.Duration(batches*waves*probeTimes*len(cfg.URLs)) * probeTimeout
	if budget < 15*time.Minute {
		return 15 * time.Minute
	}
	return budget + 2*time.Minute
}

func Summarize(name string, rounds [][]int, attempts, successes int) Metric {
	values := make([]int, 0, len(rounds))
	passes := 0
	for _, round := range rounds {
		if len(round) == 0 {
			continue
		}
		passes++
		sorted := append([]int(nil), round...)
		sort.Ints(sorted)
		values = append(values, sorted[len(sorted)/2])
	}
	if len(values) == 0 {
		return Metric{Name: name, Attempts: attempts, Successes: successes}
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	jitter := sorted[len(sorted)-1] - sorted[0]
	return Metric{Name: name, LatencyMs: sorted[len(sorted)/2], PassCount: passes, JitterMs: jitter, HealthScore: healthScore(passes, len(rounds), jitter), Attempts: attempts, Successes: successes}
}

// Eligible decides whether a benchmarked node is worth publishing. Availability
// is gated at request level so a single transient burst at probe time no longer
// discards a healthy node, while latency and jitter bounds still come from
// complete probe rounds. At least one complete round is required so the
// latency/jitter bounds are always computed from real measurements.
func Eligible(metric Metric, cfg Config) bool {
	times := cfg.ProbeTimes
	if times < 3 {
		times = 3
	}
	attempts := times * len(cfg.URLs)
	if attempts == 0 {
		return false
	}
	// ceil(attempts*2/3): a majority (two thirds) of all delay probes must
	// have succeeded before latency and jitter are even considered.
	needed := (attempts*2 + 2) / 3
	if metric.Successes < needed || metric.PassCount < 1 {
		return false
	}
	return metric.LatencyMs <= int(cfg.MaxLatency/time.Millisecond) && metric.JitterMs <= int(cfg.MaxJitter/time.Millisecond)
}

func healthScore(pass, total, jitter int) int {
	if total == 0 {
		return 0
	}
	score := pass * 100 / total
	if jitter > 0 {
		score -= jitter / 10
	}
	if score < 0 {
		return 0
	}
	return score
}

type Controller struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (c Controller) Delay(ctx context.Context, proxyName, rawURL string, timeout time.Duration) (int, error) {
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/proxies/" + url.PathEscape(proxyName) + "/delay?url=" + url.QueryEscape(rawURL) + "&timeout=" + strconv.FormatInt(timeout.Milliseconds(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("controller returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("failed to parse controller response: %w", err)
	}
	if body.Delay <= 0 {
		return 0, fmt.Errorf("controller returned an invalid delay")
	}
	return body.Delay, nil
}

func Probe(ctx context.Context, controller Controller, candidates []Candidate, cfg Config) []Metric {
	if cfg.ProbeTimes < 3 {
		cfg.ProbeTimes = 3
	}
	if len(cfg.URLs) == 0 {
		return nil
	}
	results := make([]Metric, len(candidates))
	workers := len(candidates)
	if workers > 24 {
		workers = 24
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				candidate := candidates[index]
				rounds := make([][]int, 0, cfg.ProbeTimes)
				successes := 0
				for round := 0; round < cfg.ProbeTimes; round++ {
					values := make([]int, 0, len(cfg.URLs))
					complete := true
					for _, target := range cfg.URLs {
						timeout := cfg.ProbeTimeout
						if timeout <= 0 {
							timeout = 5 * time.Second
						}
						delay, err := controller.Delay(ctx, candidate.Name, target, timeout)
						if err != nil {
							complete = false
							continue
						}
						values = append(values, delay)
						successes++
					}
					if !complete {
						// Incomplete rounds still contribute per-request
						// successes; latency and jitter only come from
						// complete rounds.
						continue
					}
					rounds = append(rounds, values)
				}
				attempts := cfg.ProbeTimes * len(cfg.URLs)
				results[index] = Summarize(candidate.Name, rounds, attempts, successes)
			}
		}()
	}
	for index := range candidates {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

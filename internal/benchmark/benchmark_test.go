package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestProbeClientTimeoutExceedsBudget keeps the probe timing invariant honest:
// the controller client deadline must stay above the per-URL delay budget so
// Mihomo's own budget, not the client, decides a failed probe. Tuning either
// value without adjusting the other breaks the reference behavior.
func TestEstimatedTimeoutReservesIsolationPass(t *testing.T) {
	cfg := DefaultConfig()
	got := EstimatedTimeout(1000, cfg)
	// Normal path: 10 batches x 5 waves x 3 rounds x 3 URLs x 2s = 15 min.
	// One full isolation pass: 100 nodes x (9 probes x 2s + 1s startup).
	// Plus the 2-minute startup/validation reserve.
	want := 15*time.Minute + 100*(9*2*time.Second+time.Second) + 2*time.Minute
	if got != want {
		t.Fatalf("EstimatedTimeout(1000) = %v, want %v", got, want)
	}
	got = EstimatedTimeout(2000, cfg)
	if got <= want {
		t.Fatalf("EstimatedTimeout(2000) = %v, want above %v", got, want)
	}
}

func TestBenchmarkRejectsMismatchedCandidateNames(t *testing.T) {
	proxies := []map[string]any{{"name": "proxy-name"}}
	candidates := []Candidate{{Name: "candidate-name"}}
	metrics, dropped, err := Benchmark(context.Background(), "unused", proxies, candidates, DefaultConfig())
	if err == nil || metrics != nil || dropped != nil {
		t.Fatalf("expected name contract error, got metrics=%v dropped=%v err=%v", metrics, dropped, err)
	}
}
func TestCheckConfigMissingCoreIsNotNodeRejection(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	err := checkConfig(context.Background(), filepath.Join(t.TempDir(), "missing-mihomo"), configPath)
	var rejected *ConfigRejectedError
	if err == nil || errors.As(err, &rejected) {
		t.Fatalf("missing core should be an engine error, got %T: %v", err, err)
	}
}
func TestProbeClientTimeoutExceedsBudget(t *testing.T) {
	budget := DefaultConfig().ProbeTimeout
	if controllerClientTimeout <= budget {
		t.Fatalf("controller client timeout %v must exceed probe budget %v", controllerClientTimeout, budget)
	}
}

func TestSummarizeAndEligibility(t *testing.T) {
	cfg := DefaultConfig()
	attempts := cfg.ProbeTimes * len(cfg.URLs) // 9 delays per candidate
	metric := Summarize("node", [][]int{{100, 120}, {150, 160}, {110, 130}}, attempts, attempts)
	if metric.LatencyMs != 130 || metric.JitterMs != 40 || metric.PassCount != 3 {
		t.Fatalf("unexpected metric: %#v", metric)
	}
	if !Eligible(metric, cfg) {
		t.Fatalf("expected eligible metric: %#v", metric)
	}
	// Losing one full round (3 of 9 probes) is probe-time noise, not node
	// quality: two complete passing rounds keep the node eligible.
	bursty := Summarize("node", [][]int{{100, 120}, {150, 160}}, attempts, attempts-3)
	if !Eligible(bursty, cfg) {
		t.Fatalf("majority-passing node must stay eligible: %#v", bursty)
	}
	// Fewer than two thirds of probes means a flaky node: not eligible.
	flaky := Summarize("node", [][]int{{100, 120}}, attempts, 5)
	if Eligible(flaky, cfg) {
		t.Fatalf("half-dead node must not pass: %#v", flaky)
	}
	// Successes without a single complete round cannot support a latency bound.
	noRound := Summarize("node", nil, attempts, attempts-3)
	if Eligible(noRound, cfg) {
		t.Fatalf("node without complete rounds must not pass: %#v", noRound)
	}
}

func TestProbeController(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/proxies/node") || r.URL.Query().Get("url") == "" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delay":120}`))
	}))
	defer server.Close()
	cfg := Config{ProbeTimes: 3, MaxLatency: time.Second, MaxJitter: time.Second, URLs: []string{"https://example.com"}}
	metrics := Probe(context.Background(), Controller{BaseURL: server.URL}, []Candidate{{Name: "node"}}, cfg)
	if len(metrics) != 1 || metrics[0].PassCount != 3 || metrics[0].LatencyMs != 120 {
		t.Fatalf("unexpected probe result: %#v", metrics)
	}
}

func TestBuildBenchmarkConfig(t *testing.T) {
	data, err := BuildBenchmarkConfig([]map[string]any{{"name": "node"}, {"name": ""}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"BENCHMARK", "DIRECT", "\"node\"", "\"unified-delay\":true", "\"tcp-concurrent\":true", "\"ipv6\":true"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
}

func TestWithPorts(t *testing.T) {
	data, err := json.Marshal(BenchmarkConfig{Proxies: []map[string]any{{"name": "node"}}})
	if err != nil {
		t.Fatal(err)
	}
	config, err := withPorts(data, 1234, 5678)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, expected := range []string{"127.0.0.1:1234", "\"mixed-port\":5678", "\"allow-lan\":false"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
}

func TestProbeToleratesFailedRound(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "failed", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"delay":100}`))
	}))
	defer server.Close()
	cfg := Config{ProbeTimes: 3, MaxLatency: time.Second, MaxJitter: time.Second, URLs: []string{"https://example.com"}}
	metrics := Probe(context.Background(), Controller{BaseURL: server.URL}, []Candidate{{Name: "node"}}, cfg)
	// One transient failure must not discard the node: 2 of 3 requests
	// succeeded and two complete rounds back the latency measurement.
	if len(metrics) != 1 || metrics[0].PassCount != 2 || !Eligible(metrics[0], cfg) {
		t.Fatalf("expected transient failure to keep the node eligible, got %#v (calls %d)", metrics, calls)
	}
}

func TestLimitedBufferConcurrent(t *testing.T) {
	buf := &limitedBuffer{limit: 1 << 20}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_, _ = buf.Write([]byte("log line\n"))
				_ = buf.String()
			}
		}()
	}
	wg.Wait()
	if len(buf.String()) > buf.limit {
		t.Fatalf("buffer exceeds limit: %d", len(buf.String()))
	}
}

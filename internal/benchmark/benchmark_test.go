package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestProbeClientTimeoutExceedsBudget keeps the probe timing invariant honest:
// the controller client deadline must stay above the per-URL delay budget so
// Mihomo's own budget, not the client, decides a failed probe. Tuning either
// value without adjusting the other breaks the reference behavior.
func TestProbeClientTimeoutExceedsBudget(t *testing.T) {
	budget := DefaultConfig().ProbeTimeout
	if controllerClientTimeout <= budget {
		t.Fatalf("controller client timeout %v must exceed probe budget %v", controllerClientTimeout, budget)
	}
}

func TestSummarizeAndEligibility(t *testing.T) {
	cfg := DefaultConfig()
	metric := Summarize("node", [][]int{{100, 120}, {150, 160}, {110, 130}})
	if metric.LatencyMs != 130 || metric.JitterMs != 40 || metric.PassCount != 3 {
		t.Fatalf("unexpected metric: %#v", metric)
	}
	if !Eligible(metric, cfg) {
		t.Fatalf("expected eligible metric: %#v", metric)
	}
	if Eligible(Summarize("node", [][]int{{100}, {}, {100}}), cfg) {
		t.Fatal("incomplete rounds must not pass")
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

func TestProbeDropsFailedRound(t *testing.T) {
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
	if len(metrics) != 1 || metrics[0].PassCount != 1 {
		t.Fatalf("expected one failed round, got %#v (calls %s)", metrics, strconv.Itoa(calls))
	}
}

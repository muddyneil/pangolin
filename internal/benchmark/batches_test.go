package benchmark

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// coreBinary resolves tools/mihomo/mihomo(.exe) relative to the module root.
// Tests that need a real Mihomo are skipped when the binary is absent so CI
// without a downloaded core still passes.
func coreBinary(t *testing.T) string {
	t.Helper()
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name = "mihomo.exe"
	}
	path := filepath.Join("..", "..", "tools", "mihomo", name)
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Skip("cannot resolve mihomo path")
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skip("tools/mihomo binary absent; skipping real-engine test")
	}
	return abs
}

func TestBenchmarkDropsOnlyRejectedNode(t *testing.T) {
	core := coreBinary(t)
	makeSS := func(name string) map[string]any {
		return map[string]any{"name": name, "type": "ss", "server": "127.0.0.1", "port": 1, "cipher": "aes-256-gcm", "password": "x"}
	}
	// vmess without alterId/cipher is rejected by Mihomo -t.
	bad := map[string]any{"name": "bad-vmess", "type": "vmess", "server": "example.com", "port": 443, "uuid": "11111111-1111-1111-1111-111111111111"}
	proxies := []map[string]any{bad, makeSS("n1"), makeSS("n2"), makeSS("n3")}
	candidates := []Candidate{{Name: "bad-vmess"}, {Name: "n1"}, {Name: "n2"}, {Name: "n3"}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	metrics, dropped, err := Benchmark(ctx, core, proxies, candidates, DefaultConfig())
	if err != nil {
		t.Fatalf("Benchmark failed: %v", err)
	}
	if len(dropped) != 1 || dropped[0] != "bad-vmess" {
		t.Fatalf("expected only bad-vmess dropped, got %v", dropped)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 healthy nodes measured, got %d", len(metrics))
	}
	// Healthy nodes are 127.0.0.1:1, so all rounds fail; the metric must still
	// exist with zero passes rather than being dropped.
	for _, name := range []string{"n1", "n2", "n3"} {
		found := false
		for _, m := range metrics {
			if m.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("healthy node %s missing from metrics", name)
		}
	}
}

func TestRunBatchAbortsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	proxies := []map[string]any{{"name": "n1", "type": "ss", "server": "127.0.0.1", "port": 1, "cipher": "aes-256-gcm", "password": "x"}}
	candidates := []Candidate{{Name: "n1"}}
	metrics, dropped, err := runBatch(ctx, "irrelevant-core", proxies, candidates, DefaultConfig())
	if err == nil || ctx.Err() == nil {
		t.Fatalf("expected canceled-context error, got metrics=%v dropped=%v err=%v", metrics, dropped, err)
	}
	if metrics != nil || dropped != nil {
		t.Fatalf("no metrics or drops expected on canceled context: %v %v", metrics, dropped)
	}
}

func TestBenchmarkHealthyBatchMeasured(t *testing.T) {
	core := coreBinary(t)
	proxies := make([]map[string]any, 0, 3)
	candidates := make([]Candidate, 0, 3)
	for i := 0; i < 3; i++ {
		name := string(rune('a' + i))
		proxies = append(proxies, map[string]any{"name": name, "type": "ss", "server": "127.0.0.1", "port": 1, "cipher": "aes-256-gcm", "password": "x"})
		candidates = append(candidates, Candidate{Name: name})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	metrics, dropped, err := Benchmark(ctx, core, proxies, candidates, DefaultConfig())
	if err != nil {
		t.Fatalf("Benchmark failed: %v", err)
	}
	if len(dropped) != 0 {
		t.Fatalf("unexpected drops: %v", dropped)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}
}

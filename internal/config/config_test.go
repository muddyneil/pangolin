package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("sources:\n  - name: test\n    primary: https://example.com/a.yaml\n    fallbacks: [\"https://example.com/b.yaml\", \"https://example.com/c.yaml\", \"https://example.com/query?values=1,2\"]\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[0].Primary != "https://example.com/a.yaml" || len(cfg.Sources[0].Fallbacks) != 3 || cfg.Sources[0].Fallbacks[2] != "https://example.com/query?values=1,2" {
		t.Fatalf("unexpected source: %#v", cfg.Sources)
	}
}

func TestInvalidSource(t *testing.T) {
	cfg := Config{Sources: []Source{{Name: "x", Primary: "ftp://example.com"}}}
	if err := cfg.Validate("config.yaml"); err == nil {
		t.Fatal("expected invalid URL")
	}
}

func TestRejectsLocalSource(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/feed", "https://10.0.0.1/feed", "http://localhost/feed", "http://[::1]/feed"} {
		t.Run(raw, func(t *testing.T) {
			cfg := Config{Sources: []Source{{Name: "x", Primary: raw}}}
			if err := cfg.Validate("config.yaml"); err == nil {
				t.Fatalf("expected local URL to be rejected: %s", raw)
			}
		})
	}
}

func TestRepoConfigLoads(t *testing.T) {
	// The repository root config.yaml is the runtime source of truth for
	// deployments; loading it in CI keeps broken configuration from passing.
	path := filepath.Join("..", "..", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("repository config.yaml not present")
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("repository config.yaml failed validation: %v", err)
	}
}

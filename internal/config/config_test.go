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

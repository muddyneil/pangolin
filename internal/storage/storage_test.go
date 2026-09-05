package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clash.yaml")
	if err := AtomicWrite(path, []byte("key: value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "key: value\n" {
		t.Fatalf("unexpected output: %q, %v", data, err)
	}
}

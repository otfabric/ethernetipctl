package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A model file above MaxModelSize is refused without reading it whole.
func TestLoadModelSize(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, []byte(strings.Repeat("#", MaxModelSize+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(big); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("model of %d octets: %v", MaxModelSize+1, err)
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("missing model loaded")
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfigPath(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(ok, []byte("mode: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigPath(ok); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	if err := validateConfigPath(""); err == nil {
		t.Fatal("empty path must be handled by caller, but empty string means no config")
	}
	cases := map[string]string{
		"/etc/passwd":          "non-yaml",
		"../secrets/env.yaml":  "traversal",
		"../../etc/shadow.yml": "traversal-yml",
		"config.txt":           "wrong extension",
	}
	for p, why := range cases {
		if err := validateConfigPath(p); err == nil {
			t.Fatalf("validateConfigPath(%q) must fail (%s)", p, why)
		}
	}
}
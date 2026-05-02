package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaultsAndNormalizesDomains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"server": {"httpAddr": ":8080"},
		"smtp": {"addr": ":2525"},
		"mail": {"domains": ["Example.COM."], "retention": "30m"},
		"access": {"mode": "public"},
		"admin": {"username": "root"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasDomain("example.com") {
		t.Fatalf("expected normalized domain, got %#v", cfg.Mail.Domains)
	}
	if cfg.IsReservedLocalPart("admin") != true {
		t.Fatal("expected default reserved local parts")
	}
}

func TestValidateRejectsInvalidAccessMode(t *testing.T) {
	cfg := Default()
	cfg.Access.Mode = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

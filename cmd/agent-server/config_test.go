package main

import (
	"os"
	"testing"
)

func TestLoadConfigDuration(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	data := []byte(
		"models:\n  default:\n    provider: openai\n    model: test\n    credential_ref: KEY\n    timeout: 60s\n",
	)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if value.Models["default"].Timeout.Seconds() != 60 {
		t.Fatalf("timeout=%v", value.Models["default"].Timeout)
	}
}

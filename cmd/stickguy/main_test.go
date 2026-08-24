package main

import (
	"encoding/json"
	"testing"
)

func TestVersionEnvelope(t *testing.T) {
	b, err := json.Marshal(versionInfo{Version: "dev", Commit: "abc", BuildTime: "now", SchemaMinimum: 1, SchemaMaximum: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["schemaMinimum"] != float64(1) || got["schemaMaximum"] != float64(1) {
		t.Fatalf("unexpected protocol range: %s", b)
	}
}

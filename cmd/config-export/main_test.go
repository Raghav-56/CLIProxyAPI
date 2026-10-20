package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportConfigPreservesSelectedAccountAndSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := "config-version: 8\nserver: {host: '127.0.0.1', port: 8317}\noauth: {auth-dir: '/old/auth'}\nrouting: {codex-oauth-auth-id: 'selected.json', session-affinity: true}\naccess: {api-keys: [synthetic-client-key]}\nmanagement: {secret-key: synthetic-management-key}\n"
	if errWrite := os.WriteFile(path, []byte(raw), 0600); errWrite != nil {
		t.Fatal(errWrite)
	}
	var output bytes.Buffer
	if errExport := exportConfig(path, &output); errExport != nil {
		t.Fatal(errExport)
	}
	var result struct {
		Routing struct {
			Selected string `json:"codex-oauth-auth-id"`
			Affinity bool   `json:"session-affinity"`
		} `json:"routing"`
		Access struct {
			Keys []string `json:"api-keys"`
		} `json:"access"`
		Management struct {
			Key string `json:"secret-key"`
		} `json:"management"`
	}
	if errDecode := json.Unmarshal(output.Bytes(), &result); errDecode != nil {
		t.Fatal(errDecode)
	}
	if result.Routing.Selected != "selected.json" || !result.Routing.Affinity || len(result.Access.Keys) != 1 || result.Access.Keys[0] != "synthetic-client-key" || result.Management.Key != "synthetic-management-key" {
		t.Fatalf("configuration fields did not survive export: %s", output.String())
	}
}

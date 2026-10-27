// SPDX-License-Identifier: Apache-2.0
// FR: Empêche les versions périmées et les réécritures de composants étrangers.
// EN: Prevents stale versions and rewriting unrelated component pins.
// Called by: go test; Calls: synchronize and isolated fixtures.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exampleFixture
// FR: Prépare cinq épingles périmées et des versions étrangères à préserver.
// EN: Prepares five stale pins alongside unrelated versions to preserve.
// Called by: tests; Calls: temporary filesystem.
func exampleFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{
		"version.go":                          "package main\nvar pluginVersion = \"1.2.3\"\n",
		"README.md":                           "The public example pins **v0.1.16**.\n      version: v0.1.16\nimage: traefik:v3.7.16\n",
		"LISEZMOI.md":                         "L’exemple public utilise la version **v0.1.16**.\n      version: v0.1.16\nimage: crowdsecurity/crowdsec:v1.8.1\n",
		"examples/compose/docker-compose.yml": "      - --experimental.plugins.parefluxgate.version=v0.1.16\nimage: redis:8.10.3-alpine\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestReleaseExamples
// FR: Contrôle les divergences sans écrire puis synchronise de manière idempotente.
// EN: Detects drift without writing, then synchronizes idempotently.
// Called by: go test; Calls: synchronize.
func TestReleaseExamples(t *testing.T) {
	root := exampleFixture(t)
	path := filepath.Join(root, "README.md")
	before, _ := os.ReadFile(path)
	if err := synchronize(root, "", false); err == nil {
		t.Fatal("stale examples accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("check wrote files")
	}
	if err := synchronize(root, "v1.2.3", true); err != nil {
		t.Fatal(err)
	}
	if err := synchronize(root, "", false); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "**v1.2.3**") || !strings.Contains(string(after), "traefik:v3.7.16") {
		t.Fatal(string(after))
	}
	compose, _ := os.ReadFile(filepath.Join(root, "examples/compose/docker-compose.yml"))
	if !strings.Contains(string(compose), "version=v1.2.3") || !strings.Contains(string(compose), "redis:8.10.3-alpine") {
		t.Fatal(string(compose))
	}
	if err := synchronize(root, "v1.2.4", true); err == nil {
		t.Fatal("version.go mismatch accepted")
	}
	if err := synchronize(root, "v1.2.3", true); err != nil {
		t.Fatal(err)
	}
	idempotent, _ := os.ReadFile(path)
	if string(idempotent) != string(after) {
		t.Fatal("rewrite not idempotent")
	}
}

// TestInvalidPinsDoNotPartiallyRewrite
// FR: Refuse une épingle absente ou dupliquée avant toute modification.
// EN: Rejects missing or duplicate pins before changing any file.
// Called by: go test; Calls: synchronize.
func TestInvalidPinsDoNotPartiallyRewrite(t *testing.T) {
	for _, content := range []string{"missing\n", "      - --experimental.plugins.parefluxgate.version=v0.1.16\n      - --experimental.plugins.parefluxgate.version=v0.1.17\n"} {
		root := exampleFixture(t)
		path := filepath.Join(root, "README.md")
		before, _ := os.ReadFile(path)
		if err := os.WriteFile(filepath.Join(root, "examples/compose/docker-compose.yml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := synchronize(root, "", true); err == nil {
			t.Fatal("invalid pins accepted")
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Fatal("partially changed source")
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie la navigation du catalogue, la préservation du code et le refus de liens cassés.
// EN: Verifies catalog navigation, code preservation and rejection of broken links.
// Called by: go test and release verification; Calls: publication link exporter.
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fixture
// FR: Crée un petit dépôt documentaire isolé, sans réseau ni données de déploiement.
// EN: Creates a small isolated documentation repository without network or deployment data.
// Called by: tests; Calls: temporary filesystem.
func fixture(t *testing.T) exporter {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string]string{"README.md": "# Start\n\n## Setup\n", "LISEZMOI.md": "# Début\n", "docs/guide.md": "# Guide\n", "docs/guide (FR).md": "# Guide français\n", ".assets/icon.svg": "<svg/>"} {
		name := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return exporter{root, "github.com/example/plugin", "v1.2.3"}
}

// TestCatalogNavigation
// FR: Garantit des liens immuables utilisables depuis un site tiers et dans les deux langues.
// EN: Ensures immutable links usable from a third-party site and in both languages.
// Called by: go test; Calls: transform, run.
func TestCatalogNavigation(t *testing.T) {
	e := fixture(t)
	document := filepath.Join(e.root, "README.md")
	source := `[Français](LISEZMOI.md)
[Setup](#setup)
[Guide](docs/guide.md "Read this")
[Guide FR](<docs/guide (FR).md>)
![Icon](.assets/icon.svg)
[Files](docs)
[ref]: docs/guide.md
<img src=".assets/icon.svg"><a href="LISEZMOI.md">FR</a>
[Old main](https://github.com/example/plugin/blob/main/LISEZMOI.md)
[Outside](https://example.org/page)
` + "`[literal](missing.md)`\n```yaml\nurl: '[not a link](missing.md)'\n```\n"
	result, err := e.transform(document, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"https://www.github.com/example/plugin/blob/v1.2.3/LISEZMOI.md", "[Setup](#setup)", "https://www.github.com/example/plugin/blob/v1.2.3/docs/guide.md", "https://www.github.com/example/plugin/blob/v1.2.3/docs/guide%20%28FR%29.md", "https://raw.githubusercontent.com/example/plugin/v1.2.3/.assets/icon.svg", "https://www.github.com/example/plugin/tree/v1.2.3/docs", "[Outside](https://example.org/page)", "`[literal](missing.md)`", "url: '[not a link](missing.md)'"} {
		if !strings.Contains(result, expected) {
			t.Errorf("missing %q in %s", expected, result)
		}
	}
	again, err := e.transform(document, result)
	if err != nil || again != result {
		t.Fatal("rewriting must be idempotent", err)
	}
	child, err := e.transform(filepath.Join(e.root, "docs/guide.md"), "[Back](../README.md#setup)")
	if err != nil || !strings.Contains(child, "/blob/v1.2.3/README.md#setup") {
		t.Fatal(child, err)
	}
}

// TestBrokenTargetsStopPublication
// FR: Empêche les 404 et les sorties du dépôt avant toute modification du paquet exporté.
// EN: Prevents missing targets and repository escapes before modifying the export package.
// Called by: go test; Calls: target and run.
func TestBrokenTargetsStopPublication(t *testing.T) {
	e := fixture(t)
	doc := filepath.Join(e.root, "README.md")
	for _, target := range []string{"missing.md", "README.md#missing", "../../outside.md", "https://raw.githubusercontent.com/example/plugin/main"} {
		if _, err := e.target(doc, target, false); err == nil {
			t.Errorf("accepted invalid target %q", target)
		}
	}
	before := "# Start\n\n[Good](LISEZMOI.md)\n[Bad](missing.md)\n"
	if err := os.WriteFile(doc, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(e.root, e.module, e.version, false); err == nil {
		t.Fatal("missing file should block publication")
	}
	actual, _ := os.ReadFile(doc)
	if string(actual) != before {
		t.Fatal("failed validation partially rewrote documents")
	}
}

// TestCheckModeAndSymlinks
// FR: Le contrôle ne modifie rien et rejette un fichier pointant hors du dépôt.
// EN: Check mode changes nothing and rejects a file pointing outside the repository.
// Called by: go test; Calls: run and target.
func TestCheckModeAndSymlinks(t *testing.T) {
	e := fixture(t)
	doc := filepath.Join(e.root, "README.md")
	if err := os.WriteFile(doc, []byte("[FR](LISEZMOI.md)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(e.root, e.module, e.version, true); err == nil {
		t.Fatal("check accepted unprepared links")
	}
	if err := run(e.root, e.module, e.version, false); err != nil {
		t.Fatal(err)
	}
	if err := run(e.root, e.module, e.version, true); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(e.root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.target(doc, "escape.md", false); err == nil {
		t.Fatal("accepted external symlink")
	}
}

// TestCodeAndIncompleteRawLinks
// FR: Préserve les exemples imbriqués et refuse les URL raw incomplètes, même en texte libre.
// EN: Preserves nested examples and rejects incomplete raw URLs even in plain text.
// Called by: go test; Calls: transform.
func TestCodeAndIncompleteRawLinks(t *testing.T) {
	e := fixture(t)
	doc := filepath.Join(e.root, "README.md")
	source := "````markdown\n```\n[not a link](missing.md)\n```\n````\n    [indented code](missing.md)\n"
	value, err := e.transform(doc, source)
	if err != nil || value != source {
		t.Fatal("code example modified", err)
	}
	for _, text := range []string{"https://raw.githubusercontent.com/example/plugin/main", "[bad](https://raw.githubusercontent.com/example/plugin/main)"} {
		if _, err := e.transform(doc, text); err == nil {
			t.Errorf("accepted incomplete raw link %q", text)
		}
	}
}

// TestOnlyCatalogReadmeChanges
// FR: Garantit que les documents ouverts sur GitHub conservent leurs liens relatifs.
// EN: Ensures documents opened on GitHub retain their relative links.
// Called by: go test; Calls: run and local filesystem.
func TestOnlyCatalogReadmeChanges(t *testing.T) {
	e := fixture(t)
	readme := filepath.Join(e.root, "README.md")
	other := filepath.Join(e.root, "LISEZMOI.md")
	before := "# Début\n[Retour](README.md)\n"
	if err := os.WriteFile(other, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("# Start\n[FR](LISEZMOI.md)\n[Start](#start)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(e.root, e.module, e.version, false); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(other)
	if err != nil || string(actual) != before {
		t.Fatal("non-catalog document changed", err)
	}
	actual, err = os.ReadFile(readme)
	if err != nil || !strings.Contains(string(actual), "blob/v1.2.3/LISEZMOI.md") || !strings.Contains(string(actual), "[Start](#start)") {
		t.Fatal("catalog links incorrect", err)
	}
}

// TestReadableGitHubLinksSurviveCatalogRewrite covers the actual external renderer rule.
// FR: Évite la conversion automatique github.com vers raw, sans casser les images.
// EN: Avoids automatic github.com-to-raw rewriting without breaking images.
// Called by: go test; Calls: target and catalog URL rule observed in its published frontend.
func TestReadableGitHubLinksSurviveCatalogRewrite(t *testing.T) {
	e := fixture(t)
	doc := filepath.Join(e.root, "README.md")
	for _, test := range []struct{ source, want string }{
		{"https://github.com/example/plugin", "https://www.github.com/example/plugin"},
		{"https://raw.githubusercontent.com/example/plugin", "https://www.github.com/example/plugin"},
		{"https://raw.githubusercontent.com/example/plugin/main/docs/guide.md", "https://www.github.com/example/plugin/blob/v1.2.3/docs/guide.md"},
		{"https://github.com/example/plugin/tree/main/docs", "https://www.github.com/example/plugin/tree/v1.2.3/docs"},
		{"https://www.github.com/example/plugin/blob/v1.2.3/docs/guide.md", "https://www.github.com/example/plugin/blob/v1.2.3/docs/guide.md"},
	} {
		got, err := e.target(doc, test.source, false)
		if err != nil || got != test.want {
			t.Fatalf("%s -> %s (%v), want %s", test.source, got, err, test.want)
		}
		if strings.HasPrefix(got, "https://github.com/") {
			t.Fatal("Traefik rewrites this host to raw")
		}
	}
	image, err := e.target(doc, "https://raw.githubusercontent.com/example/plugin/main/.assets/icon.svg", true)
	if err != nil || image != "https://raw.githubusercontent.com/example/plugin/v1.2.3/.assets/icon.svg" {
		t.Fatalf("image changed: %s (%v)", image, err)
	}
	if _, err := e.target(doc, "https://raw.githubusercontent.com/example/plugin/main/docs", true); err == nil {
		t.Fatal("image cannot reference a directory")
	}
}

// TestDocumentationTargets
// FR: Valide aussi les liens des guides secondaires sans réécrire leurs fichiers.
// EN: Also validates secondary guide links without rewriting their files.
// Called by: go test; Calls: transform and local documentation.
func TestDocumentationTargets(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "version.go"))
	if err != nil {
		t.Fatal(err)
	}
	version := regexp.MustCompile(`pluginVersion = "([0-9.]+)"`).FindStringSubmatch(string(source))
	if len(version) != 2 {
		t.Fatal("missing source version")
	}
	e := exporter{root, "github.com/vandman/pareflux-traefik-gate", "v" + version[1]}
	for _, pattern := range []string{"*.md", "docs/*.md"} {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.transform(path, string(raw)); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
	}
}

// TestUserGuidesExcludeMaintainerDetails
// FR: Empêche le retour des procédures et paramètres privés dans les guides utilisateurs.
// EN: Prevents private procedures and settings from returning to user guides.
// Called by: go test and release verification; Calls: local Markdown files.
func TestUserGuidesExcludeMaintainerDetails(t *testing.T) {
	root := filepath.Join("..", "..")
	paths := []string{filepath.Join(root, "README.md"), filepath.Join(root, "LISEZMOI.md")}
	guides, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, guides...)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, internal := range []string{"vm-runner", "OpenBao", "PAREFLUX_GITHUB_DEPLOY_KEY", "publish/v", "go mod vendor", "apply.py", "gitlab.nikolov.fr", "CI trust contract", "Contrat de confiance de la CI"} {
			if strings.Contains(string(raw), internal) {
				t.Errorf("%s contains maintainer detail %q", path, internal)
			}
		}
	}
}

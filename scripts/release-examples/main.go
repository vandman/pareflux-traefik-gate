// SPDX-License-Identifier: Apache-2.0
// FR: Synchronise les exemples du plugin avec la version source ou publiée.
// EN: Synchronizes plugin examples with the source or published version.
// Called by: CI, publish-github.sh; Calls: local files only.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var stableVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// main
// FR: Vérifie sans écrire par défaut ; la publication demande une réécriture.
// EN: Checks without writing by default; publishing explicitly requests rewriting.
// Called by: command line; Calls: synchronize.
func main() {
	root := flag.String("root", ".", "repository root")
	version := flag.String("version", "", "release tag; empty reads version.go")
	write := flag.Bool("write", false, "rewrite examples after validating every file")
	flag.Parse()
	if err := synchronize(*root, *version, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// synchronize
// FR: Contrôle les cinq épingles attendues et conserve les autres versions.
// EN: Validates the five expected pins while preserving other component versions.
// Called by: main, tests; Calls: local filesystem and regular expressions.
func synchronize(root, version string, write bool) error {
	source, err := os.ReadFile(filepath.Join(root, "version.go"))
	if err != nil {
		return err
	}
	match := regexp.MustCompile(`(?m)^var pluginVersion = "([0-9]+\.[0-9]+\.[0-9]+)"(?:[ \t]*//[^\n]*)?$`).FindStringSubmatch(string(source))
	if len(match) != 2 {
		return fmt.Errorf("version.go: expected one stable pluginVersion")
	}
	if version == "" {
		version = "v" + match[1]
	}
	if !stableVersion.MatchString(version) || version != "v"+match[1] {
		return fmt.Errorf("release version must match version.go")
	}
	changes := make(map[string]string)
	for _, spec := range []struct {
		path, pattern string
	}{
		{"README.md", `(?m)(The public example pins \*\*)v[0-9]+\.[0-9]+\.[0-9]+(\*\*)`},
		{"LISEZMOI.md", `(?m)(L’exemple public utilise la version \*\*)v[0-9]+\.[0-9]+\.[0-9]+(\*\*)`},
		{"README.md", `(?m)(      version: )v[0-9]+\.[0-9]+\.[0-9]+()$`},
		{"LISEZMOI.md", `(?m)(      version: )v[0-9]+\.[0-9]+\.[0-9]+()$`},
		{"examples/compose/docker-compose.yml", `(?m)(      - --experimental\.plugins\.parefluxgate\.version=)v[0-9]+\.[0-9]+\.[0-9]+()$`},
	} {
		path := filepath.Join(root, spec.path)
		body, ok := changes[path]
		if !ok {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			body = string(raw)
		}
		pattern := regexp.MustCompile(spec.pattern)
		matches := pattern.FindAllStringSubmatch(body, -1)
		if len(matches) != 1 {
			return fmt.Errorf("%s: expected exactly one pin for %s", spec.path, spec.pattern)
		}
		replacement := matches[0][1] + version + matches[0][2]
		if !write && matches[0][0] != replacement {
			return fmt.Errorf("%s: stale plugin pin; expected %s", spec.path, version)
		}
		changes[path] = pattern.ReplaceAllStringFunc(body, func(string) string { return replacement })
	}
	// FR: Tous les fichiers sont validés avant la première écriture.
	// EN: Every file is validated before the first write.
	if write {
		for path, body := range changes {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(body), info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	fmt.Println("release examples: five pins verified for", strings.TrimSpace(version))
	return nil
}

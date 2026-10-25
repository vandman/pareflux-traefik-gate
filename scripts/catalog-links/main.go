// SPDX-License-Identifier: Apache-2.0
// FR: Prépare les liens Markdown du paquet public pour les lecteurs du catalogue Traefik.
// EN: Prepares public-package Markdown links for readers of the Traefik catalog.
// Called by: publish-github.sh; Calls: local filesystem and standard URL parsing.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var reference = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:\s*(<[^>]+>|\S+)`)
var attribute = regexp.MustCompile(`(?i)\b(href|src)\s*=\s*(["'])([^"']*)(["'])`)
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// exporter
// FR: Résout les cibles locales sans dépendre du site qui affiche le Markdown.
// EN: Resolves local targets independently from the site displaying Markdown.
// Called by: run; Calls: target, transform.
type exporter struct{ root, module, version string }

// main
// FR: Arrête la publication si un lien local est invalide ou reste relatif.
// EN: Stops publication if a local link is invalid or remains relative.
// Called by: command line; Calls: run.
func main() {
	root := flag.String("root", ".", "exported repository root")
	module := flag.String("module", "github.com/vandman/pareflux-traefik-gate", "public GitHub module")
	version := flag.String("version", "", "immutable public release tag")
	check := flag.Bool("check", false, "validate without changing files; require publication-ready links")
	flag.Parse()
	if err := run(*root, *module, *version, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run
// FR: Vérifie et prépare uniquement le README affiché dans le catalogue Traefik.
// EN: Validates and prepares only the README displayed in the Traefik catalog.
// Called by: main, tests; Calls: transform and WriteFile.
func run(root, module, version string, check bool) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid public version")
	}
	parts := strings.Split(module, "/")
	if len(parts) != 3 || parts[0] != "github.com" || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("expected github.com/owner/repository")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	e := exporter{abs, module, version}
	changes := map[string]string{}
	count := 0
	path := filepath.Join(abs, "README.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	converted, err := e.transform(path, string(raw))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	count++
	if converted != string(raw) {
		changes[path] = converted
	}
	if check && len(changes) > 0 {
		return fmt.Errorf("%d documents still contain links requiring publication rewriting", len(changes))
	}
	for path, content := range changes {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(content), info.Mode().Perm()); err != nil {
			return err
		}
	}
	fmt.Printf("catalog links: %d documents validated, %d rewritten\n", count, len(changes))
	return nil
}

// slug
// FR: Reproduit les identifiants usuels des titres GitHub, y compris les accents.
// EN: Reproduces ordinary GitHub heading identifiers, including accented letters.
// Called by: target; Calls: Unicode character classification.
func slug(text string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(text)) {
		if unicode.IsLetter(c) || unicode.IsNumber(c) || c == '_' || c == '-' {
			b.WriteRune(c)
		} else if unicode.IsSpace(c) {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// target
// FR: Vérifie les chemins et fragments et construit un lien GitHub lié au tag publié.
// EN: Validates paths and fragments and builds a GitHub link pinned to the published tag.
// Called by: transform; Calls: Stat, EvalSymlinks and URL encoding.
func (e exporter) target(document, raw string, image bool) (string, error) {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, `\(`, "("), `\)`, ")")
	localFragment := strings.HasPrefix(raw, "#")
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid documentation target %q", raw)
	}
	if u.Scheme != "" || u.Host != "" {
		rawRoot := "https://raw.githubusercontent.com/" + strings.TrimPrefix(e.module, "github.com/")
		if strings.TrimRight(raw, "/") == rawRoot {
			return "", fmt.Errorf("raw GitHub URL requires a version and file path: %q", raw)
		}
		if strings.HasPrefix(raw, rawRoot+"/") {
			remainder := strings.TrimPrefix(u.Path, "/"+strings.TrimPrefix(e.module, "github.com/")+"/")
			pieces := strings.SplitN(remainder, "/", 2)
			if len(pieces) != 2 || pieces[0] == "" || pieces[1] == "" {
				return "", fmt.Errorf("raw GitHub URL requires a version and file path: %q", raw)
			}
		}
		prefix := "https://" + e.module + "/"
		suffix := ""
		kind := ""
		for _, candidate := range []string{"blob/main/", "tree/main/", "blob/master/", "tree/master/", "blob/" + e.version + "/", "tree/" + e.version + "/"} {
			if strings.HasPrefix(raw, prefix+candidate) {
				suffix = strings.TrimPrefix(raw, prefix+candidate)
				kind = "github"
				break
			}
		}
		rawPrefix := "https://raw.githubusercontent.com/" + strings.TrimPrefix(e.module, "github.com/") + "/"
		for _, ref := range []string{"main/", "master/", e.version + "/"} {
			if strings.HasPrefix(raw, rawPrefix+ref) {
				suffix = strings.TrimPrefix(raw, rawPrefix+ref)
				kind = "raw"
				image = true
				break
			}
		}
		if kind == "" {
			return raw, nil
		}
		u, err = url.Parse(suffix)
		if err != nil {
			return "", err
		}
		document = filepath.Join(e.root, "README.md")
	}
	relative := u.Path
	var path string
	if relative == "" {
		path = document
	} else if strings.HasPrefix(relative, "/") {
		path = filepath.Join(e.root, filepath.FromSlash(strings.TrimPrefix(relative, "/")))
	} else {
		path = filepath.Join(filepath.Dir(document), filepath.FromSlash(relative))
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(e.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("documentation target escapes repository: %q", raw)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("missing documentation target %q", raw)
	}
	resolvedRel, err := filepath.Rel(e.root, resolved)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("documentation target follows an external symlink: %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if image && info.IsDir() {
		return "", fmt.Errorf("raw image/download target must be a file: %q", raw)
	}
	if u.Fragment != "" && strings.EqualFold(filepath.Ext(path), ".md") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		found := false
		seen := map[string]int{}
		fence := ""
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if fencedLine(trimmed, &fence) {
				continue
			}
			if !strings.HasPrefix(trimmed, "#") {
				continue
			}
			title := strings.TrimLeft(trimmed, "#")
			if !strings.HasPrefix(title, " ") {
				continue
			}
			s := slug(strings.TrimSpace(strings.TrimRight(title, "#")))
			index := seen[s]
			seen[s]++
			if index > 0 {
				s = fmt.Sprintf("%s-%d", s, index)
			}
			if s == u.Fragment {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("missing documentation heading %q in %s", u.Fragment, rel)
		}
	}
	if localFragment {
		return raw, nil
	}
	segments := strings.Split(filepath.ToSlash(rel), "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	encoded := strings.Join(segments, "/")
	base := "https://" + e.module + "/blob/" + e.version + "/"
	if info.IsDir() {
		base = "https://" + e.module + "/tree/" + e.version + "/"
	}
	if image {
		base = "https://raw.githubusercontent.com/" + strings.TrimPrefix(e.module, "github.com/") + "/" + e.version + "/"
	}
	result := base + encoded
	if u.RawQuery != "" {
		result += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		result += "#" + u.EscapedFragment()
	}
	return result, nil
}

// line
// FR: Convertit liens, références et attributs HTML tout en préservant le code en ligne.
// EN: Converts links, references and HTML attributes while preserving inline code.
// Called by: transform; Calls: target.
func (e exporter) line(document, line string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(line); {
		if line[i] == '`' {
			end := i
			for end < len(line) && line[end] == '`' {
				end++
			}
			mark := line[i:end]
			close := strings.Index(line[end:], mark)
			if close < 0 {
				out.WriteString(line[i:])
				break
			}
			close += end + len(mark)
			out.WriteString(line[i:close])
			i = close
			continue
		}
		start, end := -1, -1
		image := false
		if i == 0 {
			if m := reference.FindStringSubmatchIndex(line); m != nil {
				start, end = m[2], m[3]
			}
		}
		if start < 0 && i+1 < len(line) && line[i:i+2] == "](" {
			start = i + 2
			for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
				start++
			}
			end = start
			balance := 0
			angle := start < len(line) && line[start] == '<'
			if angle {
				end = strings.IndexByte(line[start:], '>')
				if end < 0 {
					return "", fmt.Errorf("unclosed angle link")
				}
				end += start + 1
			} else {
				for end < len(line) {
					c := line[end]
					if c == '\\' && end+1 < len(line) {
						end += 2
						continue
					}
					if c == '(' {
						balance++
					}
					if c == ')' {
						if balance == 0 {
							break
						}
						balance--
					}
					if (c == ' ' || c == '\t') && balance == 0 {
						break
					}
					end++
				}
			}
			label := strings.LastIndex(line[:i], "[")
			image = label > 0 && line[label-1] == '!'
		}
		if start < 0 && (strings.HasPrefix(line[i:], "https://") || strings.HasPrefix(line[i:], "http://")) {
			end := i
			for end < len(line) && !strings.ContainsRune(" \t<>\"'", rune(line[end])) {
				end++
			}
			value, err := e.target(document, line[i:end], false)
			if err != nil {
				return "", err
			}
			out.WriteString(value)
			i = end
			continue
		}
		if start < 0 && line[i] == '<' {
			if close := strings.IndexByte(line[i:], '>'); close >= 0 {
				tag := line[i : i+close+1]
				var converted string
				var err error
				inside := tag[1 : len(tag)-1]
				if strings.HasPrefix(inside, "https://") || strings.HasPrefix(inside, "http://") {
					var value string
					value, err = e.target(document, inside, false)
					converted = "<" + value + ">"
				} else {
					converted, err = e.html(document, tag)
				}
				if err != nil {
					return "", err
				}
				out.WriteString(converted)
				i += len(tag)
				continue
			}
		}
		if start >= 0 {
			if end <= start {
				out.WriteByte(line[i])
				i++
				continue
			}
			raw := line[start:end]
			angle := strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">")
			if angle {
				raw = raw[1 : len(raw)-1]
			}
			value, err := e.target(document, raw, image)
			if err != nil {
				return "", err
			}
			out.WriteString(line[i:start])
			if angle {
				out.WriteByte('<')
			}
			out.WriteString(value)
			if angle {
				out.WriteByte('>')
			}
			i = end
			continue
		}
		out.WriteByte(line[i])
		i++
	}
	return out.String(), nil
}

// html
// FR: Réécrit les liens et images HTML sans toucher aux autres attributs.
// EN: Rewrites HTML links and images without changing other attributes.
// Called by: line; Calls: target.
func (e exporter) html(document, tag string) (string, error) {
	var failure error
	result := attribute.ReplaceAllStringFunc(tag, func(attr string) string {
		m := attribute.FindStringSubmatchIndex(attr)
		value, err := e.target(document, attr[m[6]:m[7]], strings.EqualFold(attr[m[2]:m[3]], "src"))
		if err != nil {
			failure = err
			return attr
		}
		return attr[:m[6]] + value + attr[m[7]:]
	})
	return result, failure
}

// fencedLine
// FR: Préserve les blocs imbriqués dont la clôture doit être au moins aussi longue que l'ouverture.
// EN: Preserves nested fenced blocks whose closing fence must be at least as long as the opener.
// Called by: transform and target; Calls: string helpers.
func fencedLine(line string, fence *string) bool {
	if *fence != "" {
		c := string((*fence)[0])
		if strings.HasPrefix(line, *fence) && strings.TrimSpace(strings.TrimLeft(line, c)) == "" {
			*fence = ""
		}
		return true
	}
	if len(line) < 3 || line[0] != '`' && line[0] != '~' {
		return false
	}
	count := 0
	for count < len(line) && line[count] == line[0] {
		count++
	}
	if count < 3 {
		return false
	}
	*fence = line[:count]
	return true
}

// transform
// FR: Conserve intégralement les blocs de code et ne réécrit que la documentation rendue.
// EN: Preserves fenced code verbatim and rewrites only rendered documentation.
// Called by: run, tests; Calls: line.
func (e exporter) transform(document, text string) (string, error) {
	lines := strings.Split(text, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fencedLine(trimmed, &fence) || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		value, err := e.line(document, line)
		if err != nil {
			return "", err
		}
		lines[i] = value
	}
	return strings.Join(lines, "\n"), nil
}

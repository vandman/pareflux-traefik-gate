// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie les deux références documentaires contre les vrais défauts publics.
// EN: Checks both configuration references against actual public defaults.
// Called by: go test and publication; Calls: New, reflection, documentation files.
package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestDocumentedDefaults
// FR: Détecte les options oubliées, inconnues, dupliquées ou aux défauts périmés.
// EN: Detects missing, unknown, duplicate options and stale documented defaults.
// Called by: go test; Calls: New and EN/FR Markdown references.
func TestDocumentedDefaults(t *testing.T) {
	value := reflect.ValueOf(New()).Elem()
	rows := regexp.MustCompile("(?m)^\\| `([^`]+)` \\| `([^`]+)` \\| `([^`]+)` \\|")
	for _, language := range []string{"EN", "FR"} {
		t.Run(language, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "configuration."+language+".md"))
			if err != nil {
				t.Fatal(err)
			}
			documented := make(map[string][]string)
			for _, row := range rows.FindAllStringSubmatch(string(raw), -1) {
				if _, exists := documented[row[1]]; exists {
					t.Fatalf("duplicate option %s", row[1])
				}
				documented[row[1]] = row[2:]
			}
			for i := 0; i < value.NumField(); i++ {
				field := value.Type().Field(i)
				name := strings.Split(field.Tag.Get("json"), ",")[0]
				row, exists := documented[name]
				if !exists {
					t.Errorf("missing option %s", name)
					continue
				}
				delete(documented, name)
				v := value.Field(i)
				typeName := v.Type().String()
				if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct {
					typeName = "[]object"
				}
				encoded, err := json.Marshal(v.Interface())
				if err != nil {
					t.Fatal(err)
				}
				defaultValue := string(encoded)
				if v.Kind() == reflect.Slice && v.IsNil() {
					defaultValue = "[]"
				}
				if row[0] != typeName || row[1] != defaultValue {
					t.Errorf("%s: documented %s=%s, actual %s=%s", name, row[0], row[1], typeName, defaultValue)
				}
			}
			for name := range documented {
				t.Errorf("unknown option %s", name)
			}
		})
	}
}

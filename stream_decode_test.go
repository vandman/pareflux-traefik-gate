// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie les limites de mémoire et l'intégrité des instantanés incrémentalement décodés.
// EN: Verifies memory limits and integrity of incrementally decoded snapshots.
// Called by: go test; Calls: bounded snapshot decoder.
package pareflux_traefik_gate

import (
	"errors"
	"strings"
	"testing"
)

// TestStreamDecoderRejectsAmbiguousAndTruncatedSnapshots prevents incomplete snapshot publication.
// FR: Les clés dupliquées et valeurs volumineuses doivent échouer avant le remplacement du magasin.
// EN: Duplicate keys and oversized values must fail before store replacement.
// Called by: go test; Calls: decodeDecisionSnapshot.
func TestStreamDecoderRejectsAmbiguousAndTruncatedSnapshots(t *testing.T) {
	for _, raw := range []string{`{"new":[`, `{"new":null`, `{"new":null,"new":[]}`, `{"new":[]}garbage`, `{"new":[{"scenario":"` + strings.Repeat("a", 200<<10) + `"}]}`} {
		if _, err := decodeDecisionSnapshot(strings.NewReader(raw), 4); err == nil {
			t.Fatal("invalid snapshot accepted", raw[:12])
		}
	}
	for _, raw := range []string{`{"new":null,"deleted":null}`, `{"new":[],"extra":{"field":"value"}}`} {
		if _, err := decodeDecisionSnapshot(strings.NewReader(raw), 4); err != nil {
			t.Fatal(err)
		}
	}
	valid := `{"id":1,"scope":"ip","value":"192.0.2.1","type":"ban","duration":"1h"}`
	stream, err := decodeDecisionSnapshot(strings.NewReader(`{"new":[`+valid+`,`+valid+`]}`), 1)
	if err != nil || len(stream.New) != 1 {
		t.Fatal("duplicate identity consumes capacity", err)
	}
	invalid := `{"id":2,"scope":"country","value":"FR","type":"ban","duration":"1h"}`
	stream, err = decodeDecisionSnapshot(strings.NewReader(`{"new":[`+valid+`,`+invalid+`]}`), 1)
	if err != nil || stream.skipped != 1 {
		t.Fatal("unsupported record consumes capacity", err)
	}
	another := strings.Replace(valid, `"id":1`, `"id":3`, 1)
	if _, err = decodeDecisionSnapshot(strings.NewReader(`{"new":[`+valid+`,`+another+`]}`), 1); !errors.Is(err, errDecisionCapacity) {
		t.Fatal("real capacity overflow hidden", err)
	}
}

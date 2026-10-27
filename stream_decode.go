// SPDX-License-Identifier: Apache-2.0
// FR: Décode les instantanés sans conserver leur représentation JSON complète.
// EN: Decodes snapshots without retaining the complete JSON representation.
// Called by: handleStreamCache; Calls: native JSON decoder and bounded readers.
package pareflux_traefik_gate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

var errDecisionCapacity = errors.New("decision capacity exceeded")

// decodeDecisionSnapshot bounds each decoder operation and validates the entire root before publication.
// FR: Un flux tronqué, ambigu ou surdimensionné ne remplace jamais les décisions actives.
// EN: Truncated, ambiguous or oversized input never replaces active decisions.
// Called by: handleStreamCache; Calls: native json.Decoder, io.LimitedReader.
func decodeDecisionSnapshot(reader io.Reader, capacity int) (Stream, error) {
	budget := &io.LimitedReader{R: reader, N: 64 << 10}
	decoder := json.NewDecoder(budget)
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Stream{}, fmt.Errorf("invalid snapshot root")
	}
	stream := Stream{}
	seen := map[string]bool{}
	positions := map[string]int{}
	for decoder.More() {
		budget.N = 64 << 10
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return Stream{}, fmt.Errorf("invalid or duplicate snapshot field")
		}
		seen[key] = true
		budget.N = 64 << 10
		if key != "new" && key != "deleted" {
			var ignored json.RawMessage
			if err = decoder.Decode(&ignored); err != nil {
				return Stream{}, err
			}
			continue
		}
		token, err = decoder.Token()
		if err != nil {
			return Stream{}, err
		}
		if token == nil {
			continue
		}
		if token != json.Delim('[') {
			return Stream{}, fmt.Errorf("invalid decision array")
		}
		for {
			budget.N = 64 << 10
			if !decoder.More() {
				break
			}
			var decision Decision
			if err = decoder.Decode(&decision); err != nil {
				return Stream{}, err
			}
			if key == "new" {
				if decision.Simulated || (decision.Type != "ban" && decision.Type != "captcha") {
					continue
				}
				_, prefixErr := decisionPrefix(decision)
				ttl, ttlErr := time.ParseDuration(decision.Duration)
				if prefixErr != nil || ttlErr != nil {
					stream.skipped++
					continue
				}
				if ttl <= 0 {
					continue
				}
				id := decisionIdentity(decision)
				if position, exists := positions[id]; exists {
					stream.New[position] = decision
					continue
				}
				if len(positions) >= capacity {
					return Stream{}, errDecisionCapacity
				}
				positions[id] = len(stream.New)
				stream.New = append(stream.New, decision)
			}
		}
		budget.N = 64 << 10
		token, err = decoder.Token()
		if err != nil || token != json.Delim(']') {
			return Stream{}, fmt.Errorf("incomplete decision array")
		}
	}
	budget.N = 64 << 10
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return Stream{}, fmt.Errorf("incomplete snapshot")
	}
	budget.N = 64 << 10
	if _, err = decoder.Token(); err != io.EOF {
		return Stream{}, fmt.Errorf("trailing snapshot data")
	}
	if budget.N == 0 {
		return Stream{}, fmt.Errorf("snapshot decoding budget exceeded")
	}
	return stream, nil
}

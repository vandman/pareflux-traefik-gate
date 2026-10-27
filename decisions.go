// SPDX-License-Identifier: Apache-2.0
// FR: Conserve les décisions actives par identifiant avec un index de préfixes borné.
// EN: Retains active decisions by identifier with bounded prefix lookup.
// Called by: stream synchronization, ServeHTTP; Calls: net/netip, time, sync.
package pareflux_traefik_gate

import (
	"fmt"
	"github.com/vandman/pareflux-traefik-gate/pkg/cache"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// storedDecision separates decision identity, matching prefix, and expiry.
// FR: Supprimer une décision ne supprime pas les autres blocages de la même adresse.
// EN: Deleting one decision never deletes other bans on the same address.
// Called by: decisionStore; Calls: none.
type storedDecision struct {
	prefix      netip.Prefix
	remediation string
	expires     time.Time
}

// decisionStore never evicts live bans and fails closed on unknown addresses when saturated.
// FR: La saturation est explicite et reste bloquante jusqu'à un instantané complet valide.
// EN: Saturation is explicit and stays fail-closed until a complete valid snapshot arrives.
// Called by: engineState; Calls: sync.RWMutex.
type decisionStore struct {
	mu       sync.RWMutex
	byID     map[string]storedDecision
	index    map[netip.Prefix]map[string]storedDecision
	capacity int
	overflow bool
	skipped  uint64
}

// newDecisionStore creates a bounded authoritative store, distinct from the TTL acceleration cache.
// FR: Le plafond limite la mémoire sans faire disparaître un blocage existant.
// EN: Capacity bounds memory without dropping existing bans.
// Called by: acquireEngine; Calls: none.
func newDecisionStore(capacity int) *decisionStore {
	if capacity <= 0 {
		capacity = 262144
	}
	return &decisionStore{byID: make(map[string]storedDecision), index: make(map[netip.Prefix]map[string]storedDecision), capacity: capacity}
}

// decisionIdentity uses LAPI IDs and a deterministic fallback for compatible feeds without IDs.
// FR: Le repli conserve la portée et le scénario pour éviter les collisions entre décisions.
// EN: The fallback retains scope and scenario to avoid collisions between decisions.
// Called by: apply; Calls: fmt.Sprintf.
func decisionIdentity(d Decision) string {
	if d.ID != 0 {
		return fmt.Sprintf("id:%d", d.ID)
	}
	return strings.ToLower(d.Scope) + ":" + d.Value + ":" + d.Type + ":" + d.Origin + ":" + d.Scenario
}

// decisionPrefix canonicalizes IPs, mapped IPv4 and ranges and rejects interface zones.
// FR: La recherche et l'import utilisent exactement la même représentation réseau.
// EN: Lookup and import use exactly the same network representation.
// Called by: apply; Calls: netip parsers.
func decisionPrefix(d Decision) (netip.Prefix, error) {
	if strings.EqualFold(d.Scope, "range") {
		p, err := netip.ParsePrefix(d.Value)
		if err != nil {
			return p, err
		}
		if p.Addr().Is4In6() {
			if p.Bits() < 96 {
				return netip.Prefix{}, fmt.Errorf("ambiguous mapped range")
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		return p.Masked(), nil
	}
	if d.Scope != "" && !strings.EqualFold(d.Scope, "ip") {
		return netip.Prefix{}, fmt.Errorf("unsupported scope")
	}
	a, err := netip.ParseAddr(d.Value)
	if err != nil || a.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("invalid address")
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// remove deletes exactly one indexed decision while preserving overlapping matches.
// FR: Appel sous verrou exclusif ; les autres identifiants restent actifs.
// EN: Called under the exclusive lock; other identifiers remain active.
// Called by: apply; Calls: delete.
func (s *decisionStore) remove(id string) {
	if d, ok := s.byID[id]; ok {
		delete(s.byID, id)
		delete(s.index[d.prefix], id)
		if len(s.index[d.prefix]) == 0 {
			delete(s.index, d.prefix)
		}
	}
}

// apply installs snapshots atomically; overflow keeps existing bans and refuses unknown clients.
// FR: Les expirations sont purgées avant le contrôle de capacité, sans éviction de décisions actives.
// EN: Expired entries are removed before capacity checks, without evicting live decisions.
// Called by: handleStreamCache; Calls: decisionPrefix, decisionIdentity.
func (s *decisionStore) apply(stream Stream, snapshot bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped += stream.skipped
	now := time.Now()
	additions := make(map[string]storedDecision)
	for _, d := range stream.New {
		if d.Simulated {
			continue
		}
		r := ""
		switch d.Type {
		case "ban":
			r = cache.BannedValue
		case "captcha":
			r = cache.CaptchaValue
		default:
			continue
		}
		p, err := decisionPrefix(d)
		if err != nil {
			s.skipped++
			continue
		}
		ttl, err := time.ParseDuration(d.Duration)
		if err != nil {
			s.skipped++
			continue
		}
		if ttl <= 0 {
			continue
		}
		additions[decisionIdentity(d)] = storedDecision{p, r, now.Add(ttl)}
	}
	for id, d := range s.byID {
		if !d.expires.After(now) {
			s.remove(id)
		}
	}
	if snapshot && len(additions) <= s.capacity {
		s.byID = make(map[string]storedDecision)
		s.index = make(map[netip.Prefix]map[string]storedDecision)
		s.overflow = false
	}
	for _, d := range stream.Deleted {
		s.remove(decisionIdentity(d))
	}
	for id, d := range additions {
		if _, ok := s.byID[id]; !ok && len(s.byID) >= s.capacity {
			s.overflow = true
			continue
		}
		s.remove(id)
		s.byID[id] = d
		if s.index[d.prefix] == nil {
			s.index[d.prefix] = make(map[string]storedDecision)
		}
		s.index[d.prefix][id] = d
	}
	if s.overflow {
		return fmt.Errorf("decision capacity exceeded; unknown clients are refused")
	}
	return nil
}

// lookup checks at most 33 IPv4 or 129 IPv6 prefixes; bans take precedence over captchas.
// FR: Le coût de recherche ne dépend pas du nombre d'adresses bloquées.
// EN: Lookup cost does not depend on the number of blocked addresses.
// Called by: ServeHTTP; Calls: netip.PrefixFrom.
func (s *decisionStore) lookup(raw string) (string, bool) {
	a, err := netip.ParseAddr(raw)
	if err != nil {
		return cache.BannedValue, true
	}
	a = a.Unmap()
	now := time.Now()
	result := ""
	s.mu.RLock()
	defer s.mu.RUnlock()
	for n := a.BitLen(); n >= 0; n-- {
		p := netip.PrefixFrom(a, n).Masked()
		for _, d := range s.index[p] {
			if d.expires.After(now) {
				if d.remediation == cache.BannedValue {
					return cache.BannedValue, true
				}
				result = d.remediation
			}
		}
	}
	if result != "" {
		return result, true
	}
	if s.overflow {
		return cache.BannedValue, true
	}
	return "", false
}

// skippedCount reads cumulative malformed or unsupported entries without exposing feed contents.
// FR: Les décisions ignorées n'activent jamais le refus global réservé à la saturation.
// EN: Skipped decisions never trigger the global refusal reserved for saturation.
// Called by: stream synchronization and dashboard; Calls: RWMutex.
func (s *decisionStore) skippedCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skipped
}

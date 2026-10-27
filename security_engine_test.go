// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie les blocages de plage, la saturation, l'isolation et l'arrêt des moteurs.
// EN: Verifies prefix bans, saturation, engine isolation and worker shutdown.
// Called by: go test; Calls: real HTTP fixtures, New, decisionStore.
package pareflux_traefik_gate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/vandman/pareflux-traefik-gate/pkg/cache"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestAuthoritativeDecisionsKeepOverlappingBans exercises canonical prefixes and ID deletion.
// FR: Un renouvellement et une suppression ne doivent jamais lever un autre blocage valide.
// EN: Renewals and deletions must never lift another valid ban.
// Called by: go test; Calls: apply, lookup.
func TestAuthoritativeDecisionsKeepOverlappingBans(t *testing.T) {
	s := newDecisionStore(10)
	if err := s.apply(Stream{New: []Decision{{ID: 1, Scope: "range", Value: "192.0.2.123/24", Type: "ban", Duration: "1h"}, {ID: 2, Scope: "ip", Value: "::ffff:192.0.2.1", Type: "captcha", Duration: "1h"}, {ID: 3, Scope: "range", Value: "2001:db8::123/48", Type: "ban", Duration: "1h"}}}, true); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"192.0.2.1", "192.0.2.255", "::ffff:192.0.2.2", "2001:db8:0:ffff::1"} {
		if v, ok := s.lookup(ip); !ok || v != cache.BannedValue {
			t.Fatalf("range not applied: %s %s", ip, v)
		}
	}
	if err := s.apply(Stream{Deleted: []Decision{{ID: 2}}}, false); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.lookup("192.0.2.1"); v != cache.BannedValue {
		t.Fatal("IP deletion lifted overlapping range")
	}
	if err := s.apply(Stream{Deleted: []Decision{{ID: 1}}}, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.lookup("192.0.2.1"); ok {
		t.Fatal("deleted range retained")
	}
	if _, ok := s.lookup("2001:db8:1::1"); ok {
		t.Fatal("range extended beyond prefix")
	}
}

// TestDecisionsBeyondOldLRUCapacity keeps more than 65,536 active bans and fails closed on overflow.
// FR: Reproduit l'attaque d'éviction sans autoriser une adresse déjà bannie.
// EN: Reproduces eviction pressure without allowing an already banned address.
// Called by: go test; Calls: apply, lookup.
func TestDecisionsBeyondOldLRUCapacity(t *testing.T) {
	s := newDecisionStore(66000)
	items := make([]Decision, 66000)
	for n := range items {
		items[n] = Decision{ID: n + 1, Scope: "ip", Value: fmt.Sprintf("2001:db8::%x:%x", n>>16, n&65535), Type: "ban", Duration: "1h"}
	}
	if err := s.apply(Stream{New: items}, true); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.lookup("2001:db8::"); v != cache.BannedValue {
		t.Fatal("oldest active ban evicted")
	}
	if err := s.apply(Stream{New: []Decision{{ID: 99999, Scope: "ip", Value: "198.51.100.1", Type: "ban", Duration: "1h"}}}, false); err == nil {
		t.Fatal("capacity silently exceeded")
	}
	if v, _ := s.lookup("203.0.113.1"); v != cache.BannedValue {
		t.Fatal("overflow failed open")
	}
	if err := s.apply(Stream{New: items[:1]}, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.lookup("203.0.113.1"); ok {
		t.Fatal("complete snapshot did not recover overflow")
	}
}

// TestEngineReloadIsolationAndRelease rotates credentials while another route still uses the old engine.
// FR: Vérifie la première synchronisation, les références et la libération finale sans singleton périmé.
// EN: Checks first synchronization, references and final release without a stale singleton.
// Called by: go test; Calls: New, registry, HTTP fixture.
func TestEngineReloadIsolationAndRelease(t *testing.T) {
	var calls atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("startup") != "true" {
			t.Error("missing initial/full snapshot")
		}
		id := 1
		ip := "192.0.2.1"
		if r.Header.Get("X-Api-Key") == "second" {
			id = 2
			ip = "198.51.100.1"
		}
		fmt.Fprintf(w, `{"new":[{"id":%d,"scope":"ip","value":%q,"type":"ban","duration":"1h"}]}`, id, ip)
	}))
	defer api.Close()
	endpoint, _ := url.Parse(api.URL)
	cfg := CreateConfig()
	cfg.Enabled = true
	cfg.CrowdsecMode = "stream"
	cfg.CrowdsecLapiHost = endpoint.Host
	cfg.CrowdsecLapiKey = "first"
	cfg.MetricsUpdateIntervalSeconds = 0
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	h1, err := New(ctx1, http.NotFoundHandler(), cfg, "one")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := New(ctx2, http.NotFoundHandler(), cfg, "two")
	if err != nil {
		t.Fatal(err)
	}
	first := h1.(*Bouncer)
	second := h2.(*Bouncer)
	if first.engine != second.engine || calls.Load() != 1 {
		t.Fatal("matching routes duplicated engine")
	}
	cfg.CrowdsecLapiKey = "second"
	h3, err := New(ctx3, http.NotFoundHandler(), cfg, "rotated")
	if err != nil {
		t.Fatal(err)
	}
	rotated := h3.(*Bouncer)
	if rotated.engine == first.engine {
		t.Fatal("key rotation reused old engine")
	}
	if _, ok := rotated.engine.decisions.lookup("192.0.2.1"); ok {
		t.Fatal("old decision crossed credentials")
	}
	if _, ok := rotated.engine.decisions.lookup("198.51.100.1"); !ok {
		t.Fatal("new engine not synchronized")
	}
	cancel1()
	time.Sleep(10 * time.Millisecond)
	if first.engine.ctx.Err() != nil {
		t.Fatal("one route stopped a shared engine")
	}
	cancel2()
	select {
	case <-first.engine.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("unused engine not stopped")
	}
	if rotated.engine.ctx.Err() != nil {
		t.Fatal("old release stopped new engine")
	}
}

// TestEngineFingerprintIncludesLimits separates stores and grace policies after reconfiguration.
// FR: Un nouveau plafond ne réutilise pas un magasin de l'ancienne capacité.
// EN: A new capacity cannot reuse a store with the old limit.
// Called by: go test; Calls: engineFingerprint.
func TestEngineFingerprintIncludesLimits(t *testing.T) {
	cfg := CreateConfig()
	initial, err := engineFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.StreamDecisionCapacity++
	changed, err := engineFingerprint(cfg)
	if err != nil || changed == initial {
		t.Fatal("capacity ignored", err)
	}
	cfg.StreamDecisionCapacity--
	cfg.FailureGraceSeconds++
	changed, err = engineFingerprint(cfg)
	if err != nil || changed == initial {
		t.Fatal("grace ignored", err)
	}
}

// TestMalformedDecisionsCannotBanUnrelatedClients preserves valid entries around malformed feed items.
// FR: Les portées futures, IP et durées invalides ne déclenchent jamais une saturation.
// EN: Future scopes, invalid IPs and durations never trigger saturation.
// Called by: go test; Calls: authoritative decision apply and lookup.
func TestMalformedDecisionsCannotBanUnrelatedClients(t *testing.T) {
	s := newDecisionStore(4)
	stream := Stream{New: []Decision{
		{ID: 1, Scope: "country", Value: "FR", Type: "ban", Duration: "1h"},
		{ID: 2, Scope: "ip", Value: "invalid", Type: "ban", Duration: "1h"},
		{ID: 3, Scope: "ip", Value: "192.0.2.3", Type: "ban", Duration: "invalid"},
		{ID: 4, Scope: "ip", Value: "192.0.2.4", Type: "ban", Duration: "1h"},
	}}
	for n := 0; n < 2; n++ {
		if err := s.apply(stream, true); err != nil {
			t.Fatal(err)
		}
		if _, blocked := s.lookup("198.51.100.1"); blocked {
			t.Fatal("invalid decision enabled global refusal")
		}
		if _, blocked := s.lookup("192.0.2.4"); !blocked {
			t.Fatal("valid decision discarded")
		}
	}
	if s.skippedCount() != 6 {
		t.Fatal("missing skipped entry accounting", s.skippedCount())
	}
}

// TestSnapshotAtDefaultCapacityExceedsLegacyResponseLimit verifies the complete default-size feed.
// FR: Un flux de 262144 décisions ne doit pas échouer sur l'ancienne limite de 16 Mio.
// EN: A 262144-decision feed must not fail at the previous 16 MiB response limit.
// Called by: go test; Calls: real HTTP snapshot ingestion and authoritative lookup.
func TestSnapshotAtDefaultCapacityExceedsLegacyResponseLimit(t *testing.T) {
	items := make([]Decision, 262144)
	for n := range items {
		items[n] = Decision{ID: n + 1, Scope: "ip", Value: fmt.Sprintf("2001:db8::%x:%x", n>>16, n&65535), Type: "ban", Duration: "1h", Scenario: strings.Repeat("s", 64)}
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Stream{New: items}) }))
	defer api.Close()
	endpoint, _ := url.Parse(api.URL)
	b := &Bouncer{crowdsecScheme: "http", crowdsecHost: endpoint.Host, crowdsecPath: "/", crowdsecStreamRoute: "decisions/stream", crowdsecHeader: "X-Api-Key", httpClient: api.Client(), engine: &engineState{ctx: context.Background(), decisions: newDecisionStore(0)}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := handleStreamCache(b); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"2001:db8::", "2001:db8::3:ffff"} {
		if _, ok := b.engine.decisions.lookup(ip); !ok {
			t.Fatal("snapshot incomplete", ip)
		}
	}
}

// TestRangeCapabilityRequiresExactFreshEngine rejects unrelated health and saturated stores.
// FR: Le menu ne confirme jamais un autre moteur que celui explicitement configuré.
// EN: The menu never confirms an engine other than the explicitly configured one.
// Called by: go test; Calls: engine registry capability lookup.
func TestRangeCapabilityRequiresExactFreshEngine(t *testing.T) {
	b := &Bouncer{crowdsecMode: "stream", updateInterval: 10, engine: &engineState{refs: 1, lastStreamSuccess: time.Now().Unix(), decisions: newDecisionStore(4)}}
	engines.Lock()
	engines.values["range-fixture"] = b
	engines.Unlock()
	defer func() { engines.Lock(); delete(engines.values, "range-fixture"); engines.Unlock() }()
	if rangeEnforcementReady("") || rangeEnforcementReady("different") {
		t.Fatal("unrelated engine proves capability")
	}
	if !rangeEnforcementReady("range-fixture") {
		t.Fatal("fresh matching engine rejected")
	}
	atomic.StoreInt64(&b.engine.lastStreamSuccess, time.Now().Add(-time.Hour).Unix())
	if rangeEnforcementReady("range-fixture") {
		t.Fatal("stale engine accepted")
	}
	atomic.StoreInt64(&b.engine.lastStreamSuccess, time.Now().Unix())
	b.engine.decisions.mu.Lock()
	b.engine.decisions.overflow = true
	b.engine.decisions.mu.Unlock()
	if rangeEnforcementReady("range-fixture") {
		t.Fatal("saturated engine accepted")
	}
}

// TestDiscoveryFingerprintResolvesInheritedAppsecTransport matches pre- and post-construction options.
// FR: Le menu désactivé doit retrouver le moteur dont le schéma AppSec a été hérité à la construction.
// EN: The disabled menu must find the engine whose AppSec scheme was inherited during construction.
// Called by: go test; Calls: engineFingerprint.
func TestDiscoveryFingerprintResolvesInheritedAppsecTransport(t *testing.T) {
	cfg := CreateConfig()
	cfg.CrowdsecAppsecEnabled = true
	cfg.CrowdsecAppsecScheme = ""
	cfg.CrowdsecLapiKey = "fixture"
	before, err := engineFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.CrowdsecAppsecScheme = cfg.CrowdsecLapiScheme
	cfg.CrowdsecAppsecKey = cfg.CrowdsecLapiKey
	after, err := engineFingerprint(cfg)
	if err != nil || before != after {
		t.Fatal("inherited options change discovery identity", err)
	}
}

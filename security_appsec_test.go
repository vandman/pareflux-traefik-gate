// SPDX-License-Identifier: Apache-2.0
// FR: Reproduit les contournements de protocole, les corps excessifs et l'annulation d'inspection.
// EN: Reproduces protocol bypasses, oversized bodies and inspection cancellation.
// Called by: go test; Calls: AppSec fixture, request contexts, memory accounting.
package pareflux_traefik_gate

import (
	"bytes"
	"context"
	"errors"
	"github.com/vandman/pareflux-traefik-gate/pkg/configuration"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestAppsecRejectsSpoofedStreamingHeaders requires protocol-valid upgrades for body exemptions.
// FR: Un simple en-tête ne doit plus rendre un corps invisible.
// EN: A single header must no longer hide a body.
// Called by: go test; Calls: shouldInspectAppsecBody.
func TestAppsecRejectsSpoofedStreamingHeaders(t *testing.T) {
	b := &Bouncer{appsecBodyLimit: 1024}
	for _, header := range []string{"Upgrade", "Content-Type"} {
		r := httptest.NewRequest("POST", "http://example/", strings.NewReader("attack"))
		r.Header.Set(header, map[string]string{"Upgrade": "websocket", "Content-Type": "application/grpc"}[header])
		if !shouldInspectAppsecBody(b, r) {
			t.Fatal("spoof bypassed body inspection", header)
		}
	}
	r := httptest.NewRequest("GET", "http://example/", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "keep-alive, Upgrade")
	r.Header.Set("Sec-WebSocket-Version", "13")
	r.Header.Set("Sec-WebSocket-Key", "MDEyMzQ1Njc4OWFiY2RlZg==")
	if shouldInspectAppsecBody(b, r) {
		t.Fatal("valid bodyless websocket handshake buffered")
	}
}

// TestAppsecLimitCancellationAndCapacity keeps reservations until replay closes and rejects unsafe loads.
// FR: Contrôle le refus, la restitution mémoire et la propagation d'une annulation cliente.
// EN: Checks rejection, memory release and propagation of client cancellation.
// Called by: go test; Calls: appsecQuery, replay Close, HTTP context.
func TestAppsecLimitCancellationAndCapacity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.WriteHeader(200) }))
	defer upstream.Close()
	b := testAppsecBouncer(t, upstream.URL, 4)
	before := atomic.LoadInt64(&inspectionBytes)
	r := httptest.NewRequest("POST", "http://example/", bytes.NewReader([]byte("12345")))
	if err := appsecQuery(b, "192.0.2.1", r); !errors.Is(err, errBodyTooLarge) {
		t.Fatal("oversized body accepted", err)
	}
	if atomic.LoadInt64(&inspectionBytes) != before {
		t.Fatal("reservation leaked after body rejection")
	}
	r = httptest.NewRequest("POST", "http://example/", strings.NewReader("1234"))
	if err := appsecQuery(b, "192.0.2.1", r); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&inspectionBytes) <= before {
		t.Fatal("reservation released before replay")
	}
	releaseInspectionBody(r)
	releaseInspectionBody(r)
	waitInspectionBytes(t, before)
	if atomic.LoadInt64(&inspectionBytes) != before {
		t.Fatal("replay accounting incorrect")
	}
	if !reserveInspection(maxInspectionBytes) {
		t.Fatal("cannot reserve test budget")
	}
	r = httptest.NewRequest("POST", "http://example/", strings.NewReader("x"))
	err := appsecQuery(b, "192.0.2.1", r)
	atomic.AddInt64(&inspectionBytes, -maxInspectionBytes)
	if !errors.Is(err, errInspectionCapacity) {
		t.Fatal("budget exhaustion accepted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = httptest.NewRequest("POST", "http://example/", strings.NewReader("x")).WithContext(ctx)
	if err := appsecQuery(b, "192.0.2.1", r); !errors.Is(err, context.Canceled) {
		t.Fatal("client cancellation ignored", err)
	}
}

// TestAppsecReadBudgetInterruptsSlowBody verifies the read itself belongs to the inspection deadline.
// FR: Le délai ne commence pas seulement après le préchargement du corps.
// EN: The deadline does not start only after preloading the body.
// Called by: go test; Calls: pipe Close on context cancellation.
func TestAppsecReadBudgetInterruptsSlowBody(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	b := &Bouncer{appsecBodyLimit: 1024, appsecTimeout: 30 * time.Millisecond}
	r := httptest.NewRequest("POST", "http://example/", reader)
	start := time.Now()
	if err := appsecQuery(b, "192.0.2.1", r); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("slow body did not expire", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("body deadline exceeded")
	}
}

// TestUploadExceptionIsScoped keeps sensitive routes strict while allowing declared upload methods.
// FR: Une route, un domaine ou une méthode différents ne bénéficient pas de l'exception.
// EN: Different routes, hosts or methods do not inherit the exception.
// Called by: go test; Calls: compileBodyExceptions, partialBodyAllowed.
func TestUploadExceptionIsScoped(t *testing.T) {
	rules, err := compileBodyExceptions([]configuration.AppsecBodyException{{Host: "registry.example", PathRegexp: "^/v2/.*/blobs/uploads/", Methods: []string{"PATCH", "PUT"}}})
	if err != nil {
		t.Fatal(err)
	}
	b := &Bouncer{appsecBodyExceptions: rules}
	for _, tc := range []struct {
		method, url string
		allow       bool
	}{{"PATCH", "https://registry.example/v2/team/repo/blobs/uploads/id", true}, {"POST", "https://registry.example/users/sign_in", false}, {"GET", "https://registry.example/v2/team/repo/blobs/uploads/id", false}, {"PATCH", "https://other.example/v2/team/repo/blobs/uploads/id", false}, {"PATCH", "https://registry.example/v2/a/../../users/sign_in", false}} {
		r := httptest.NewRequest(tc.method, tc.url, nil)
		if b.partialBodyAllowed(r) != tc.allow {
			t.Fatal("incorrect upload policy", tc)
		}
	}
}

// TestAppsecOutageGraceSurvivesTraffic avoids consuming the whole grace in a busy second.
// FR: La grâce finit après le délai et le seuil ; un refus de sécurité ne bénéficie jamais de cette grâce.
// EN: Grace ends after both time and failure thresholds; security denials never receive grace.
// Called by: go test; Calls: appsecQuery with real HTTP failures.
func TestAppsecOutageGraceSurvivesTraffic(t *testing.T) {
	var status atomic.Int64
	status.Store(503)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(status.Load())) }))
	defer upstream.Close()
	b := testAppsecBouncer(t, upstream.URL, 1024)
	b.engine = &engineState{lastAppsecSuccess: time.Now().Unix()}
	b.updateMaxFailure = 3
	b.failureGraceSeconds = 60
	b.appsecUnreachableBlock = true
	b.appsecFailureBlock = true
	for n := 0; n < 20; n++ {
		if err := appsecQuery(b, "192.0.2.1", httptest.NewRequest("GET", "http://example/", nil)); err != nil {
			t.Fatal("traffic exhausted time grace", err)
		}
	}
	atomic.StoreInt64(&b.engine.lastAppsecSuccess, time.Now().Add(-time.Minute-time.Second).Unix())
	if err := appsecQuery(b, "192.0.2.1", httptest.NewRequest("GET", "http://example/", nil)); err == nil {
		t.Fatal("expired grace remained open")
	}
	atomic.StoreInt64(&b.engine.lastAppsecSuccess, time.Now().Unix())
	status.Store(403)
	if err := appsecQuery(b, "192.0.2.1", httptest.NewRequest("GET", "http://example/", nil)); err == nil {
		t.Fatal("security denial received outage grace")
	}
}

// TestEntrypointAndRouterInspectOnce verifies shared policies do not consume the request twice.
// FR: La protection d'entrypoint ne double pas l'inspection d'un routeur déjà protégé.
// EN: Entrypoint protection does not duplicate inspection on an already protected router.
// Called by: go test; Calls: New, ServeHTTP and a counting AppSec fixture.
func TestEntrypointAndRouterInspectOnce(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	endpoint, _ := url.Parse(upstream.URL)
	cfg := CreateConfig()
	cfg.Enabled = true
	cfg.CrowdsecMode = "appsec"
	cfg.CrowdsecLapiKey = "fixture"
	cfg.CrowdsecAppsecEnabled = true
	cfg.CrowdsecAppsecHost = endpoint.Host
	cfg.CrowdsecAppsecScheme = "http"
	cfg.MetricsUpdateIntervalSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { data, _ := io.ReadAll(r.Body); w.Write(data) })
	inner, err := New(ctx, backend, cfg, "router")
	if err != nil {
		t.Fatal(err)
	}
	outer, err := New(ctx, inner, cfg, "entrypoint")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://example/", strings.NewReader("body preserved"))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	outer.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "body preserved" || calls.Load() != 1 {
		t.Fatalf("status=%d body=%q inspections=%d", w.Code, w.Body.String(), calls.Load())
	}
}

// waitInspectionBytes waits for native pipe writers to relinquish their buffer references.
// FR: La libération est asynchrone et doit rester indépendante du backend.
// EN: Release is asynchronous and must remain independent of the backend.
// Called by: regression tests; Calls: atomic accounting.
func waitInspectionBytes(t *testing.T, expected int64) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for atomic.LoadInt64(&inspectionBytes) != expected && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if got := atomic.LoadInt64(&inspectionBytes); got != expected {
		t.Fatalf("reserved=%d expected=%d", got, expected)
	}
}

// TestInspectionPrefixReleaseAndClientQuota reproduces small bodies and slow upload tails.
// FR: Le préfixe libère mémoire et quota avant de lire la suite ou d'attendre la réponse.
// EN: Prefix consumption frees memory and quota before reading the tail or awaiting the response.
// Called by: go test; Calls: appsecQuery, native replay, per-client quota.
func TestInspectionPrefixReleaseAndClientQuota(t *testing.T) {
	waitInspectionBytes(t, 0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.WriteHeader(200) }))
	defer upstream.Close()
	b := testAppsecBouncer(t, upstream.URL, 1<<20)
	r := httptest.NewRequest("POST", "http://example/", strings.NewReader("tiny"))
	if err := appsecQuery(b, "192.0.2.1", r); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&inspectionBytes); got != 4 {
		t.Fatalf("small body reserves %d bytes", got)
	}
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		t.Fatal(err)
	}
	waitInspectionBytes(t, 0)
	releaseInspectionBody(r)

	// FR: La suite du corps reste bloquée alors que le préfixe est déjà consommé.
	// EN: The body tail stays stalled while the prefix is already consumed.
	b.appsecBodyLimit = 4
	b.appsecBodyLimitAction = "partial"
	reader, writer := io.Pipe()
	r = httptest.NewRequest("POST", "http://example/", reader)
	go func() { _, _ = writer.Write([]byte("12345")) }()
	if err := appsecQuery(b, "192.0.2.1", r); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(r.Body, make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	waitInspectionBytes(t, 0)
	writer.Close()
	releaseInspectionBody(r)
	releases := []func(){}
	for n := 0; n < 4; n++ {
		release, ok := acquireInspectionClient("2001:db8::1", 4)
		if !ok {
			t.Fatal("quota prematurely exhausted")
		}
		releases = append(releases, release)
	}
	if _, ok := acquireInspectionClient("2001:db8::99", 4); ok {
		t.Fatal("IPv6 rotation bypassed quota")
	}
	other, ok := acquireInspectionClient("2001:db8:0:1::1", 4)
	if !ok {
		t.Fatal("unrelated client blocked")
	}
	other()
	for _, release := range releases {
		release()
		release()
	}
}

// TestConnectedTimeoutAndPayloadFailureNeverReceiveGrace keeps slow or failing payloads closed.
// FR: Une connexion établie et un HTTP 500 ne prouvent pas une panne d'infrastructure.
// EN: An established connection and HTTP 500 do not prove an infrastructure outage.
// Called by: go test; Calls: real delayed AppSec HTTP fixture.
func TestConnectedTimeoutAndPayloadFailureNeverReceiveGrace(t *testing.T) {
	for _, status := range []int{200, 500} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			if status == 200 {
				time.Sleep(80 * time.Millisecond)
			}
			w.WriteHeader(status)
		}))
		b := testAppsecBouncer(t, upstream.URL, 1024)
		b.appsecTimeout = 20 * time.Millisecond
		b.engine = &engineState{lastAppsecSuccess: time.Now().Unix()}
		b.failureGraceSeconds = 60
		b.appsecUnreachableBlock = false
		b.appsecFailureBlock = false
		r := httptest.NewRequest("POST", "http://example/", strings.NewReader("test"))
		if err := appsecQuery(b, "192.0.2.1", r); !errors.Is(err, errInspectionUnavailable) {
			t.Fatalf("unsafe grace status=%d error=%v", status, err)
		}
		releaseInspectionBody(r)
		upstream.Close()
	}
	waitInspectionBytes(t, 0)
}

// deadlineRecorder captures supported HTTP read deadlines without exposing socket internals.
// FR: Vérifie qu'aucune échéance nulle ne supprime la protection du proxy.
// EN: Verifies no zero deadline removes proxy protection.
// Called by: deadline regression test; Calls: httptest.ResponseRecorder.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

// SetReadDeadline records each deadline supplied through ResponseController.
// FR: Le double de test simule uniquement une capacité HTTP réellement prise en charge.
// EN: The test double only simulates a supported HTTP capability.
// Called by: ResponseController; Calls: append.
func (r *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.deadlines = append(r.deadlines, deadline)
	return nil
}

// TestInspectionRestoresFiniteReadDeadline prevents slow upload tails becoming unlimited.
// FR: La durée totale du corps reste finie après la fin de l'inspection.
// EN: The total body read duration remains finite after inspection completes.
// Called by: go test; Calls: handleNextServeHTTP with a supported response controller.
func TestInspectionRestoresFiniteReadDeadline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.WriteHeader(200) }))
	defer upstream.Close()
	b := testAppsecBouncer(t, upstream.URL, 1024)
	b.appsecEnabled = true
	b.appsecReadTimeout = time.Minute
	b.next = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.WriteHeader(200) })
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	b.handleNextServeHTTP(w, httptest.NewRequest("POST", "http://example/", strings.NewReader("test")), "192.0.2.1")
	if len(w.deadlines) != 2 || w.deadlines[0].IsZero() || w.deadlines[1].IsZero() || !w.deadlines[1].After(w.deadlines[0]) {
		t.Fatal("finite deadline lost", w.deadlines)
	}
	waitInspectionBytes(t, 0)
}

// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie le menu natif sans altérer les blocages, redirections, actifs ou flux.
// EN: Verifies native navigation without changing bans, redirects, assets or streams.
// Called by: go test; Calls: New and the dashboard middleware.
package pareflux_traefik_gate

import (
	"context"
	"fmt"
	"github.com/vandman/pareflux-traefik-gate/pkg/configuration"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestDashboardMenuBoundsAndIsolation covers response eligibility, limits and original routing.
// FR: La sécurité passe avant la transformation et le service natif reste l'unique amont.
// EN: Security precedes transformation and the native service remains the only upstream.
// Called by: go test; Calls: newDashboardMenu.
func TestDashboardMenuBoundsAndIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, host, path, mime, encoding, body string
		status                                 int
		flush, inject                          bool
	}{
		{name: "native HTML", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: "<body>Traefik</body>", status: 200, inject: true},
		{name: "SSO redirect", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: "<body>SSO</body>", status: 302},
		{name: "ban", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: "<body>Forbidden</body>", status: 403},
		{name: "wrong host", host: "unrelated.example", path: "/dashboard/", mime: "text/html", body: "<body>Unrelated</body>", status: 200},
		{name: "asset", host: "traefik.example", path: "/dashboard/assets/index.js", mime: "text/javascript", body: "native", status: 200},
		{name: "API", host: "traefik.example", path: "/api/http/routers", mime: "application/json", body: "[]", status: 200},
		{name: "compressed", host: "traefik.example", path: "/dashboard/", mime: "text/html", encoding: "gzip", body: "opaque bytes", status: 200},
		{name: "large", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: strings.Repeat("a", dashboardBodyLimit) + "</body>", status: 200},
		{name: "stream", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: "<body>stream</body>", status: 200, flush: true},
		{name: "duplicate", host: "traefik.example", path: "/dashboard/", mime: "text/html", body: `<body><nav id="pareflux-dashboard-menu"></nav></body>`, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", tc.mime)
				w.Header().Set("ETag", "original")
				w.Header().Set("Content-Length", fmt.Sprint(len(tc.body)))
				if tc.encoding != "" {
					w.Header().Set("Content-Encoding", tc.encoding)
				}
				w.WriteHeader(tc.status)
				half := len(tc.body) / 2
				w.Write([]byte(tc.body[:half]))
				if tc.flush {
					w.(http.Flusher).Flush()
				}
				w.Write([]byte(tc.body[half:]))
			})
			menu, err := newDashboardMenu(next, "traefik.example", "/pareflux/")
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
			out := httptest.NewRecorder()
			menu.ServeHTTP(out, req)
			if out.Code != tc.status || calls != 1 {
				t.Fatal("original response changed", out.Code, calls)
			}
			if tc.inject {
				if !strings.Contains(out.Body.String(), `href="/pareflux/"`) || out.Header().Get("ETag") != "" || out.Header().Get("Content-Length") != "" {
					t.Fatal("menu missing or stale validators")
				}
			} else if out.Body.String() != tc.body || out.Header().Get("ETag") != "original" {
				t.Fatal("noneligible response changed")
			}
		})
	}
}

// TestDashboardMenuConfigurationAndRedirect checks safe targets, dedicated mode and same-origin canonicalization.
// FR: Le chemin sans barre finale garde sa requête et ne peut rediriger vers un autre site.
// EN: The slashless path keeps its query and cannot redirect to another site.
// Called by: go test; Calls: New.
func TestDashboardMenuConfigurationAndRedirect(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "//evil.example/x", "/\\evil.example", "https://user:password@example/x"} {
		if _, err := newDashboardMenu(http.NotFoundHandler(), "traefik.example", link); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	cfg := CreateConfig()
	cfg.DashboardHost = "traefik.example"
	cfg.DashboardLinkURL = "/pareflux/"
	menu, err := New(context.Background(), http.NotFoundHandler(), cfg, "menu")
	if err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	menu.ServeHTTP(out, httptest.NewRequest("GET", "https://traefik.example/dashboard?view=all", nil))
	if out.Code != 308 || out.Header().Get("Location") != "/dashboard/?view=all" {
		t.Fatal(out.Code, out.Header())
	}
	cfg.Enabled = true
	if _, err = New(context.Background(), http.NotFoundHandler(), cfg, "menu"); err == nil {
		t.Fatal("security mode combined with menu")
	}
}

// TestDashboardIntegrationHealthAndExchange verifies live URL registration and degraded navigation.
// FR: Une page SSO 200 ne prouve pas la santé de Control ; une panne garde un lien averti.
// EN: An SSO page with status 200 does not prove Control health; an outage keeps a warned link.
// Called by: go test; Calls: plugin registration and private health probe.
func TestDashboardIntegrationHealthAndExchange(t *testing.T) {
	ready := true
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ready {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ready"}`))
		} else {
			w.Write([]byte("<html>SSO login</html>"))
		}
	}))
	defer health.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<body>Native Traefik</body>"))
	})
	handler, err := newDashboardIntegration(next, "health.example", "/pareflux/", health.URL, strings.Repeat("k", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	menu := handler.(*dashboardMenu)
	public := httptest.NewRecorder()
	menu.ServeHTTP(public, httptest.NewRequest("GET", "https://health.example/dashboard/", nil))
	if strings.Contains(public.Body.String(), `data-state="available"`) {
		t.Fatal(public.Body.String())
	}
	private, err := newDashboardIntegration(next, "health.example", "/pareflux/", health.URL, strings.Repeat("k", 32), true)
	if err != nil {
		t.Fatal(err)
	}
	registration := httptest.NewRecorder()
	requestBody := `{"control_url":"https://health.example/pareflux/","nonce":"0123456789abcdef"}`
	signed := httptest.NewRequest("POST", "http://private/.well-known/pareflux", strings.NewReader(requestBody))
	signed.RemoteAddr = "127.0.0.1:1234"
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	signed.Header.Set("X-Pareflux-Time", stamp)
	signed.Header.Set("X-Pareflux-Signature", discoverySignature(strings.Repeat("k", 32), "request", stamp, []byte(requestBody)))
	private.ServeHTTP(registration, signed)
	if registration.Code != 200 || !strings.Contains(registration.Body.String(), `"dashboard_url":"https://health.example/dashboard/"`) {
		t.Fatal(registration.Code, registration.Body.String())
	}
	ready = false
	menu.registration.mu.Lock()
	menu.registration.healthAt = time.Time{}
	menu.registration.mu.Unlock()
	public = httptest.NewRecorder()
	request := httptest.NewRequest("GET", "https://health.example/dashboard/", nil)
	request.Header.Set("Accept-Language", "fr-FR")
	menu.ServeHTTP(public, request)
	if !strings.Contains(public.Body.String(), `data-state="unavailable"`) || !strings.Contains(public.Body.String(), "ne répond pas") || !strings.Contains(public.Body.String(), `href="https://health.example/pareflux/"`) {
		t.Fatal(public.Body.String())
	}
	bad := httptest.NewRecorder()
	private.ServeHTTP(bad, httptest.NewRequest("POST", "http://private/.well-known/pareflux", strings.NewReader(`{"control_url":"https://other.example/pareflux/","nonce":"0123456789abcdef"}`)))
	if bad.Code != 403 {
		t.Fatal("unsigned discovery was accepted")
	}
}

// TestDashboardProtectionProof rejects browser-forged claims and requires recent engine evidence.
// FR: Un réglage ou en-tête ne peut pas rendre la protection verte sans middleware actif.
// EN: Configuration or headers cannot turn protection green without an active middleware.
// Called by: go test; Calls: dashboard status and request-local context proof.
func TestDashboardProtectionProof(t *testing.T) {
	handler, err := newDashboardMenu(http.NotFoundHandler(), "proof.example", "/console/")
	if err != nil {
		t.Fatal(err)
	}
	menu := handler.(*dashboardMenu)
	req := httptest.NewRequest("GET", "https://proof.example/dashboard/pareflux-status", nil)
	req.Header.Set("X-Pareflux-Protected", "true")
	if menu.status(req).Protected {
		t.Fatal("client forged protection")
	}
	engine := &engineState{lastStreamSuccess: time.Now().Unix(), isCrowdsecStreamHealthy: 1}
	proof := &gateRequestProof{gate: &Bouncer{engine: engine, enabled: true, crowdsecMode: configuration.StreamMode, updateInterval: 15}}
	req = req.WithContext(context.WithValue(req.Context(), gateContextKey{}, proof))
	if !menu.status(req).Protected {
		t.Fatal("active protection not recognized")
	}
	proof.gate.appsecEnabled = true
	if menu.status(req).Protected {
		t.Fatal("AppSec without confirmation")
	}
	proof.appsecConfirmed = true
	if !menu.status(req).Protected {
		t.Fatal("confirmed AppSec missing")
	}
	atomic.StoreInt64(&engine.lastStreamSuccess, time.Now().Add(-time.Hour).Unix())
	if menu.status(req).Protected {
		t.Fatal("stale engine shown as protected")
	}
}

// TestDashboardProbeDoesNotHoldRegistrationLock keeps navigation and replacement announcements independent.
// FR: La sonde lente ne sérialise pas le dashboard et son résultat périmé ne remplace pas le nouveau.
// EN: A slow probe does not serialize dashboard loads and its stale result cannot overwrite the replacement.
// Called by: go test; Calls: readiness HTTP fixtures and registration state.
func TestDashboardProbeDoesNotHoldRegistrationLock(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte(`{"status":"not-ready"}`))
	}))
	defer upstream.Close()
	fresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"status":"ready"}`)) }))
	defer fresh.Close()
	state := &dashboardRegistration{healthURL: upstream.URL, expires: time.Now().Add(time.Minute)}
	menu := &dashboardMenu{registration: state}
	done := make(chan bool, 1)
	go func() { done <- menu.ready(context.Background()) }()
	<-entered
	started := time.Now()
	for i := 0; i < 10; i++ {
		if menu.ready(context.Background()) {
			t.Error("unverified health reported ready")
		}
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Error("loads blocked on health probe")
	}
	state.mu.Lock()
	state.healthURL = fresh.URL
	state.healthGeneration++
	state.healthPending = false
	state.healthAt = time.Time{}
	state.mu.Unlock()
	if !menu.ready(context.Background()) {
		t.Error("new registration could not probe independently")
	}
	close(release)
	if <-done {
		t.Error("stale probe reported ready")
	}
	if !menu.ready(context.Background()) {
		t.Error("stale probe overwrote new result")
	}
}

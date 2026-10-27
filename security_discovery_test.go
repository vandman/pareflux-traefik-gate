// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie les signatures privées, le rejeu et l'expiration malgré une sonde saine.
// EN: Checks private signatures, replay protection and expiry despite a healthy probe.
// Called by: go test; Calls: dashboard registration and request fixtures.
package pareflux_traefik_gate

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDiscoveryRejectsForgeryReplayAndExpiredLinks binds the announcement to the private signed channel.
// FR: Une sonde saine ne prolonge jamais une annonce expirée.
// EN: A healthy probe never extends an expired announcement.
// Called by: go test; Calls: register, status, ready.
func TestDiscoveryRejectsForgeryReplayAndExpiredLinks(t *testing.T) {
	key := strings.Repeat("s", 32)
	handler, err := newDashboardIntegration(http.NotFoundHandler(), "expiry.example", "", "http://127.0.0.1:1/readyz", key, true)
	if err != nil {
		t.Fatal(err)
	}
	menu := handler.(*dashboardMenu)
	body := `{"control_url":"https://control.example/","nonce":"unique-nonce-0123456789"}`
	makeRequest := func(peer string, stamp string) *http.Request {
		r := httptest.NewRequest("POST", "http://private/.well-known/pareflux", strings.NewReader(body))
		r.RemoteAddr = peer
		r.Header.Set("X-Pareflux-Time", stamp)
		r.Header.Set("X-Pareflux-Signature", discoverySignature(key, "request", stamp, []byte(body)))
		return r
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	for _, r := range []*http.Request{makeRequest("198.51.100.1:1", stamp), makeRequest("127.0.0.1:1", strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10))} {
		w := httptest.NewRecorder()
		menu.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("invalid announcement accepted", w.Code)
		}
	}
	w := httptest.NewRecorder()
	menu.ServeHTTP(w, makeRequest("127.0.0.1:1", stamp))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	menu.ServeHTTP(w, makeRequest("127.0.0.1:1", stamp))
	if w.Code != 403 {
		t.Fatal("signed announcement replay accepted")
	}
	menu.registration.mu.Lock()
	menu.registration.expires = time.Now().Add(-time.Second)
	menu.registration.healthAt = time.Now()
	menu.registration.healthy = true
	menu.registration.mu.Unlock()
	request := httptest.NewRequest("GET", "https://expiry.example/dashboard/", nil)
	status := menu.status(request)
	if status.Connected || status.URL != "" {
		t.Fatal("stale healthy link exposed", status)
	}
	public, err := newDashboardIntegration(http.NotFoundHandler(), "expiry.example", "", "", key, false)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	public.ServeHTTP(w, makeRequest("127.0.0.1:1", stamp))
	if w.Code != 403 {
		t.Fatal("public middleware accepted registration")
	}
}

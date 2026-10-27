// SPDX-License-Identifier: Apache-2.0
// FR: Vérifie les redirections locales, la taille du formulaire et l'IP envoyée au fournisseur.
// EN: Checks local redirects, form size and the IP sent to the provider.
// Called by: go test; Calls: real HTTP captcha provider fixture.
package captcha

import (
	"github.com/vandman/pareflux-traefik-gate/pkg/cache"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// TestCaptchaValidationBoundary checks successful challenges cannot redirect off-origin.
// FR: Les formulaires trop grands ne déclenchent pas d'appel fournisseur.
// EN: Oversized forms never trigger provider requests.
// Called by: go test; Calls: ServeHTTP, fixture provider and bounded cache.
func TestCaptchaValidationBoundary(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil || r.PostForm.Get("remoteip") != "192.0.2.1" {
			t.Error("missing verified remoteip", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage := &cache.Client{}
	storage.New(logger, false, "", "", "")
	c := &Client{log: logger, cacheClient: storage, httpClient: upstream.Client(), infoProvider: &infoProvider{validate: upstream.URL, response: "proof"}, gracePeriodSeconds: 60}
	for _, path := range []string{"/safe?x=1", "//evil.example/path", "/%5cevil.example/path"} {
		r := httptest.NewRequest("POST", "https://console.example"+path, strings.NewReader(url.Values{"proof": {"fixture"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r, "192.0.2.1")
		location := w.Header().Get("Location")
		parsed, err := url.Parse(location)
		if w.Code != 302 || err != nil || parsed.Host != "" || parsed.Scheme != "" || strings.HasPrefix(location, "//") {
			t.Fatalf("unsafe redirect %q status %d", location, w.Code)
		}
	}
	before := calls.Load()
	r := httptest.NewRequest("POST", "https://console.example/", strings.NewReader("proof="+strings.Repeat("a", 20<<10)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r, "192.0.2.1")
	if w.Code != 400 || calls.Load() != before {
		t.Fatal("oversized form reached provider")
	}
}

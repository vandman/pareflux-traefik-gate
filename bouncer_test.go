// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux derived source file
// FR: Fichier dérivé du plugin CrowdSec Traefik v1.6.0 et adapté à PareFlux
//
//	sous Apache-2.0 sans dépendance GPL-3.0.
//
// EN: File derived from CrowdSec Traefik Plugin v1.6.0 and adapted for
//
//	PareFlux under Apache-2.0 without a GPL-3.0 dependency.
//
// Called by: PareFlux Gate runtime or tests
// Calls: package-specific collaborators
// /////////////////////////////////////////////////////////////////////////////
package pareflux_traefik_gate //nolint:revive,stylecheck

import (
	"bytes"
	"context"
	htmltemplate "html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	cache "github.com/vandman/pareflux-traefik-gate/pkg/cache"
	configuration "github.com/vandman/pareflux-traefik-gate/pkg/configuration"
)

func TestServeHTTP(t *testing.T) {
	cfg := CreateConfig()
	cfg.CrowdsecLapiKey = "test"

	ctx := context.Background()
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

	handler, err := New(ctx, next, cfg, "demo-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)
}

func TestHandleBanServeHTTPWithDifferentMethods(t *testing.T) {
	html := "<html>You are banned</html>"
	banTemplate, _ := htmltemplate.New("html").Parse(html)
	tests := []struct {
		name              string
		method            string
		banTemplate       *htmltemplate.Template
		expectBodyContent bool
	}{
		{
			name:              "GET request should have body with template",
			method:            http.MethodGet,
			banTemplate:       banTemplate,
			expectBodyContent: true,
		},
		{
			name:              "HEAD request should NOT have body even with template",
			method:            http.MethodHead,
			banTemplate:       banTemplate,
			expectBodyContent: false,
		},
		{
			name:              "POST request should have body with template",
			method:            http.MethodPost,
			banTemplate:       banTemplate,
			expectBodyContent: true,
		},
		{
			name:              "PUT request should have body with template",
			method:            http.MethodPut,
			banTemplate:       banTemplate,
			expectBodyContent: true,
		},
		{
			name:              "DELETE request should have body with template",
			method:            http.MethodDelete,
			banTemplate:       banTemplate,
			expectBodyContent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bouncer := &Bouncer{
				remediationStatusCode:   http.StatusForbidden,
				remediationCustomHeader: "X-Test-Remediation",
				banTemplate:             tt.banTemplate,
			}

			rw := httptest.NewRecorder()
			req := &http.Request{Method: tt.method}
			bouncer.handleBanServeHTTP(rw, req, "0.0.0.0", "TEST")

			// Check status code
			if rw.Code != http.StatusForbidden {
				t.Errorf("Expected status code 403, got %d", rw.Code)
			}

			// Check custom header
			headerValue := rw.Header().Get("X-Test-Remediation")
			if headerValue != "ban" {
				t.Errorf("Expected header X-Test-Remediation to be 'ban', got %s", headerValue)
			}
			if requestHeader := req.Header.Get("X-Test-Remediation"); requestHeader != "ban" {
				t.Errorf("Expected internal request marker to be 'ban', got %s", requestHeader)
			}

			// Check body content
			body := rw.Body.String()
			hasBodyContent := len(body) > 0

			if hasBodyContent != tt.expectBodyContent {
				t.Errorf("Method %s: expected body content: %v, got body content: %v (body: %q)",
					tt.method, tt.expectBodyContent, hasBodyContent, body)
			}

			// If we expect body content, verify it matches template
			if tt.expectBodyContent && body != html {
				t.Errorf("Expected body %q, got %q", html, body)
			}
		})
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestStreamFailurePolicy
// FR: Vérifie que la perte du flux LAPI est ouverte par défaut et ne devient
//
//	bloquante qu'avec l'option explicite.
//
// EN: Verifies that a lost LAPI stream fails open by default and blocks only
//
//	with the explicit option.
//
// Called by: go test
// Calls: shouldBlockOnStreamFailure
// /////////////////////////////////////////////////////////////////////////////
func TestStreamFailurePolicy(t *testing.T) {
	atomic.StoreUint32(&isCrowdsecStreamHealthy, 0)
	t.Cleanup(func() {
		atomic.StoreUint32(&isCrowdsecStreamHealthy, 1)
	})

	if shouldBlockOnStreamFailure(&Bouncer{}) {
		t.Fatal("stream failure must fail open by default")
	}
	if !shouldBlockOnStreamFailure(&Bouncer{streamUnreachableBlock: true}) {
		t.Fatal("explicit fail-closed setting must block")
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestCAPIAuthenticationFailureIsNotRecursive
// FR: Vérifie qu'un refus de la route de connexion CAPI est retourné après
//
//	une seule requête au lieu de relancer getToken sans limite.
//
// EN: Verifies that a rejected CAPI login is returned after one request
//
//	instead of recursively invoking getToken without a bound.
//
// Called by: go test
// Calls: getToken
// /////////////////////////////////////////////////////////////////////////////
func TestCAPIAuthenticationFailureIsNotRecursive(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		attempt := atomic.AddInt32(&requests, 1)
		if attempt <= 3 {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	bouncer := &Bouncer{
		crowdsecScheme: endpoint.Scheme,
		crowdsecHost:   endpoint.Host,
		crowdsecMode:   configuration.AloneMode,
		crowdsecHeader: crowdsecCapiHeader,
		httpClient:     server.Client(),
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if err := getToken(bouncer); err == nil {
		t.Fatal("rejected CAPI login unexpectedly succeeded")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("CAPI login requests = %d, want 1", got)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestCAPIRefreshPreservesRequest
// FR: Vérifie que l'unique reprise après renouvellement conserve la méthode,
//
//	le corps et utilise le nouveau jeton.
//
// EN: Verifies that the single retry after refresh preserves the method and
//
//	body and uses the new token.
//
// Called by: go test
// Calls: crowdsecQuery
// /////////////////////////////////////////////////////////////////////////////
func TestCAPIRefreshPreservesRequest(t *testing.T) {
	payload := []byte(`{"metric":"value"}`)
	var protectedRequests int32
	var methods, bodies, authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/" + crowdsecCapiLoginRoute:
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"code":200,"token":"fresh-token"}`))
		case "/protected":
			methods = append(methods, req.Method)
			body, _ := io.ReadAll(req.Body)
			bodies = append(bodies, string(body))
			authorizations = append(authorizations, req.Header.Get(crowdsecCapiHeader))
			if atomic.AddInt32(&protectedRequests, 1) == 1 {
				rw.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = rw.Write([]byte("ok"))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	bouncer := &Bouncer{
		crowdsecScheme:    endpoint.Scheme,
		crowdsecHost:      endpoint.Host,
		crowdsecMode:      configuration.AloneMode,
		crowdsecHeader:    crowdsecCapiHeader,
		crowdsecKey:       "expired-token",
		crowdsecMachineID: "machine",
		crowdsecPassword:  "password",
		httpClient:        server.Client(),
		log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body, err := crowdsecQuery(bouncer, server.URL+"/protected", payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("response = %q, want ok", body)
	}
	if len(methods) != 2 || methods[0] != http.MethodPost || methods[1] != http.MethodPost {
		t.Fatalf("methods = %#v, want two POST requests", methods)
	}
	if len(bodies) != 2 || bodies[0] != string(payload) || bodies[1] != string(payload) {
		t.Fatalf("bodies = %#v, want preserved payload", bodies)
	}
	if len(authorizations) != 2 || authorizations[0] != "expired-token" || authorizations[1] != "fresh-token" {
		t.Fatalf("authorizations = %#v, want expired then fresh token", authorizations)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestRemediationMarkerCannotBeForged
// FR: Vérifie que le marqueur présenté par un client est supprimé avant toute
//
//	décision PareFlux.
//
// EN: Verifies that a client-supplied marker is removed before any PareFlux
//
//	decision.
//
// Called by: go test
// Calls: sanitizeRemediationMarker
// /////////////////////////////////////////////////////////////////////////////
func TestRemediationMarkerCannotBeForged(t *testing.T) {
	bouncer := &Bouncer{remediationCustomHeader: "X-PareFlux-Remediation"}
	req := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	req.Header.Set("X-PareFlux-Remediation", "ban")

	bouncer.sanitizeRemediationMarker(req)

	if marker := req.Header.Get("X-PareFlux-Remediation"); marker != "" {
		t.Fatalf("client marker was retained: %q", marker)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// TestAppsecBodyHandling
// FR: Vérifie la conservation des corps HTTP inspectés et le contournement du
//
//	préchargement pour gRPC sans désactiver l'appel AppSec.
//
// EN: Verifies preservation of inspected HTTP bodies and bypass of body
//
//	preloading for gRPC without disabling the AppSec call.
//
// Called by: go test
// Calls: appsecQuery
// /////////////////////////////////////////////////////////////////////////////
func TestAppsecBodyHandling(t *testing.T) {
	t.Run("bounded inspection preserves the original body", func(t *testing.T) {
		var inspected []byte
		var authorization, cookie, safeHeader string
		appsec := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			inspected, _ = io.ReadAll(req.Body)
			authorization = req.Header.Get("Authorization")
			cookie = req.Header.Get("Cookie")
			safeHeader = req.Header.Get("X-Safe-Metadata")
			rw.WriteHeader(http.StatusOK)
		}))
		defer appsec.Close()

		bouncer := testAppsecBouncer(t, appsec.URL, 5)
		payload := []byte("hello-world")
		req := httptest.NewRequest(http.MethodPost, "https://example.test/upload", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer private-token")
		req.Header.Set("Cookie", "session=private")
		req.Header.Set("X-Safe-Metadata", "retained")
		if err := appsecQuery(bouncer, "192.0.2.1", req); err != nil {
			t.Fatal(err)
		}
		forwarded, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(inspected, payload[:5]) {
			t.Fatalf("inspected body = %q, want %q", inspected, payload[:5])
		}
		if !bytes.Equal(forwarded, payload) {
			t.Fatalf("forwarded body = %q, want %q", forwarded, payload)
		}
		if authorization != "" || cookie != "" || safeHeader != "retained" {
			t.Fatalf("unexpected AppSec headers: authorization=%q cookie=%q safe=%q", authorization, cookie, safeHeader)
		}
	})

	t.Run("grpc body is not consumed", func(t *testing.T) {
		var appsecMethod string
		appsec := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			appsecMethod = req.Method
			rw.WriteHeader(http.StatusOK)
		}))
		defer appsec.Close()

		bouncer := testAppsecBouncer(t, appsec.URL, 1024)
		payload := []byte("stream-frame")
		req := httptest.NewRequest(http.MethodPost, "https://example.test/grpc", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/grpc+proto")
		if err := appsecQuery(bouncer, "192.0.2.1", req); err != nil {
			t.Fatal(err)
		}
		forwarded, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if appsecMethod != http.MethodGet {
			t.Fatalf("AppSec method = %s, want GET without buffered body", appsecMethod)
		}
		if !bytes.Equal(forwarded, payload) {
			t.Fatalf("forwarded body = %q, want %q", forwarded, payload)
		}
	})

	t.Run("binary body is not consumed", func(t *testing.T) {
		var appsecMethod string
		appsec := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			appsecMethod = req.Method
			rw.WriteHeader(http.StatusOK)
		}))
		defer appsec.Close()

		bouncer := testAppsecBouncer(t, appsec.URL, 1024)
		bouncer.appsecBodyExcludedTypes = []string{"application/octet-stream", "image/"}
		payload := []byte("opaque-binary-payload")
		req := httptest.NewRequest(http.MethodPut, "https://example.test/upload", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/octet-stream")
		if err := appsecQuery(bouncer, "192.0.2.1", req); err != nil {
			t.Fatal(err)
		}
		forwarded, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if appsecMethod != http.MethodGet {
			t.Fatalf("AppSec method = %s, want GET without buffered body", appsecMethod)
		}
		if !bytes.Equal(forwarded, payload) {
			t.Fatalf("forwarded body = %q, want %q", forwarded, payload)
		}
	})

	t.Run("image content type prefix is excluded", func(t *testing.T) {
		bouncer := &Bouncer{
			appsecBodyLimit:         1024,
			appsecBodyExcludedTypes: []string{"image/"},
		}
		req := httptest.NewRequest(http.MethodPost, "https://example.test/photo", bytes.NewReader([]byte("jpeg")))
		req.Header.Set("Content-Type", "image/jpeg; charset=binary")
		if shouldInspectAppsecBody(bouncer, req) {
			t.Fatal("image body should not be inspected")
		}
	})
}

// /////////////////////////////////////////////////////////////////////////////
// testAppsecBouncer
// FR: Construit un Bouncer minimal relié à un serveur AppSec de test.
// EN: Builds a minimal Bouncer connected to a test AppSec server.
// Called by: TestAppsecBodyHandling
// Calls: url.Parse and httptest client
// /////////////////////////////////////////////////////////////////////////////
func testAppsecBouncer(t *testing.T, serverURL string, bodyLimit int64) *Bouncer {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	return &Bouncer{
		appsecScheme:           parsed.Scheme,
		appsecHost:             parsed.Host,
		appsecPath:             "/",
		appsecBodyLimit:        bodyLimit,
		httpAppsecClient:       http.DefaultClient,
		log:                    slog.New(slog.NewTextHandler(io.Discard, nil)),
		appsecFailureBlock:     true,
		appsecUnreachableBlock: true,
	}
}

func TestCaptchaMethodBasedLogic(t *testing.T) {
	tests := []struct {
		name              string
		method            string
		remediation       string
		expectBanFallback bool
	}{
		{
			name:              "GET with captcha remediation should allow captcha",
			method:            http.MethodGet,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
		{
			name:              "HEAD with captcha remediation should fallback to ban",
			method:            http.MethodHead,
			remediation:       cache.CaptchaValue,
			expectBanFallback: true,
		},
		{
			name:              "POST with captcha remediation should allow captcha",
			method:            http.MethodPost,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
		{
			name:              "PUT with captcha remediation should allow captcha",
			method:            http.MethodPut,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
		{
			name:              "DELETE with captcha remediation should allow captcha",
			method:            http.MethodDelete,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
		{
			name:              "PATCH with captcha remediation should allow captcha",
			method:            http.MethodPatch,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
		{
			name:              "OPTIONS with captcha remediation should allow captcha",
			method:            http.MethodOptions,
			remediation:       cache.CaptchaValue,
			expectBanFallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the core logic: captcha is served for all methods except HEAD
			shouldUseCaptcha := tt.remediation == cache.CaptchaValue && tt.method != http.MethodHead

			if shouldUseCaptcha == tt.expectBanFallback {
				t.Errorf("Method %s with %s remediation: expected ban fallback %v, but logic would use captcha %v",
					tt.method, tt.remediation, tt.expectBanFallback, shouldUseCaptcha)
			}
		})
	}
}

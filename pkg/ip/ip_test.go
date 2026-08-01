// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux forwarded identity security tests
// FR: Vérifie qu'un client ne peut pas remplacer son adresse réseau par un
//
//	en-tête X-Forwarded-For non approuvé.
//
// EN: Verifies that a client cannot replace its network address with an
//
//	untrusted X-Forwarded-For header.
//
// Called by: go test
// Calls: NewChecker, GetRemoteIP
// /////////////////////////////////////////////////////////////////////////////
package ip

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func TestGetRemoteIPIgnoresForwardedHeaderWithoutTrustedProxy(t *testing.T) {
	checker, err := NewChecker(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "https://example.test/", nil)
	request.RemoteAddr = "198.51.100.20:443"
	request.Header.Set("X-Forwarded-For", "192.0.2.99")

	got, err := GetRemoteIP(request, &PoolStrategy{Checker: checker}, "X-Forwarded-For")
	if err != nil {
		t.Fatal(err)
	}
	if got != "198.51.100.20" {
		t.Fatalf("untrusted header replaced peer address: %s", got)
	}
}

func TestGetRemoteIPUsesHeaderOnlyFromTrustedPeer(t *testing.T) {
	checker, err := NewChecker(slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "https://example.test/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Forwarded-For", "192.0.2.99, 10.0.0.3")

	got, err := GetRemoteIP(request, &PoolStrategy{Checker: checker}, "X-Forwarded-For")
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.0.2.99" {
		t.Fatalf("unexpected forwarded client address: %s", got)
	}
}

func TestGetRemoteIPRejectsMalformedTrustedChain(t *testing.T) {
	checker, err := NewChecker(slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "https://example.test/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Forwarded-For", "not-an-ip")

	if _, err := GetRemoteIP(request, &PoolStrategy{Checker: checker}, "X-Forwarded-For"); err == nil {
		t.Fatal("malformed forwarded chain was accepted")
	}
}

// SPDX-License-Identifier: Apache-2.0
// FR: Reproduit les blocages des GET et le gaspillage mémoire des corps lents.
// EN: Reproduces blocked GETs and memory waste from slow request bodies.
// Called by: go test; Calls: real HTTP fixture, body reader and quotas.
package pareflux_traefik_gate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBodyQuotaDoesNotThrottleMetadata keeps browser GETs independent of upload reservations.
// FR: Les GET HTTP/2 vides restent inspectés même lorsque quatre corps occupent le quota.
// EN: Empty HTTP/2 GETs remain inspected while four bodies occupy the quota.
// Called by: go test; Calls: appsecQuery, bounded client reservations.
func TestBodyQuotaDoesNotThrottleMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	b := testAppsecBouncer(t, upstream.URL, 1<<20)
	releases := []func(){}
	for n := 0; n < 4; n++ {
		release, ok := acquireInspectionClient("192.0.2.99", 4)
		if !ok {
			t.Fatal("quota fixture failed")
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", "http://example/", nil)
			r.Body = io.NopCloser(strings.NewReader(""))
			r.ProtoMajor = 2
			r.ContentLength = 0
			if err := appsecQuery(b, "192.0.2.99", r); err != nil {
				t.Error("bodyless request throttled", err)
			}
		}()
	}
	wg.Wait()
}

// TestSlowBodyReservesOnlyOneSmallChunk limits idle known and unknown body reservations.
// FR: Un corps annoncé volumineux ne réserve plus un mégaoctet sans envoyer ses données.
// EN: A claimed large body no longer reserves a megabyte without sending data.
// Called by: go test; Calls: io.Pipe, inspection cancellation and accounting.
func TestSlowBodyReservesOnlyOneSmallChunk(t *testing.T) {
	waitInspectionBytes(t, 0)
	for _, length := range []int64{-1, 1 << 20} {
		reader, writer := io.Pipe()
		b := &Bouncer{appsecBodyLimit: 1 << 20, appsecTimeout: time.Second}
		ctx, cancel := context.WithCancel(context.Background())
		r := httptest.NewRequest("POST", "http://example/", reader).WithContext(ctx)
		r.ContentLength = length
		done := make(chan struct{})
		go func() { defer close(done); _ = appsecQuery(b, "192.0.2.100", r) }()
		deadline := time.Now().Add(time.Second)
		for atomic.LoadInt64(&inspectionBytes) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if used := atomic.LoadInt64(&inspectionBytes); used < 1 || used > 8<<10 {
			t.Fatalf("idle body reserves %d bytes", used)
		}
		_, _ = writer.Write([]byte("small payload"))
		cancel()
		writer.Close()
		<-done
		waitInspectionBytes(t, 0)
	}
}

// TestIPv6ParentQuotaCannotConsumeAllReaders preserves workers for another allocation.
// FR: La rotation des /64 d'un même /48 ne consomme pas les 64 lecteurs globaux.
// EN: Rotating /64s within one /48 cannot consume all 64 global readers.
// Called by: go test; Calls: hierarchical client quota.
func TestIPv6ParentQuotaCannotConsumeAllReaders(t *testing.T) {
	releases := []func(){}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for n := 0; n < 16; n++ {
		release, ok := acquireInspectionClient(fmt.Sprintf("2001:db8:1:%x::1", n), 4)
		if !ok {
			t.Fatal("parent quota too early")
		}
		releases = append(releases, release)
	}
	if _, ok := acquireInspectionClient("2001:db8:1:99::1", 4); ok {
		t.Fatal("parent quota bypassed")
	}
	release, ok := acquireInspectionClient("2001:db8:2::1", 4)
	if !ok {
		t.Fatal("unrelated parent blocked")
	}
	release()
}

// TestReaderAdmissionAndCleanup bounds readers without the old 64-client outage.
// FR: Des corps bloqués laissent passer un corps sain, puis le plafond ferme l'admission et l'annulation libère tout.
// EN: Stalled bodies leave room for a healthy body, then the ceiling rejects admission and cancellation releases all resources.
// Called by: go test; Calls: bounded reader, native pipes, accounting and client quotas.
func TestReaderAdmissionAndCleanup(t *testing.T) {
	waitInspectionBytes(t, 0)
	if cap(inspectionReads) != 1024 || len(inspectionReads) != 0 {
		t.Fatal("unexpected reader ceiling or retained readers")
	}
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	writers := []*io.PipeWriter{}
	defer func() {
		cancel()
		for _, writer := range writers {
			writer.Close()
		}
		wg.Wait()
		waitInspectionBytes(t, 0)
		deadline := time.Now().Add(3 * time.Second)
		for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(inspectionReads) != 0 || runtime.NumGoroutine() > before+4 {
			t.Error("reader workers or cleanup goroutines retained after cancellation")
		}
		inspectionClients.Lock()
		defer inspectionClients.Unlock()
		if len(inspectionClients.counts) != 0 {
			t.Error("client quotas retained after cancellation", inspectionClients.counts)
		}
	}()
	// FR: Attend l'admission effective, sans dépendre de la vitesse de l'ordonnanceur.
	// EN: Waits for actual admission without depending on scheduler speed.
	// Called by: this test; Calls: bounded channel length and clock.
	waitReaders := func(expected int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for len(inspectionReads) != expected && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(inspectionReads) != expected {
			t.Fatalf("wanted %d readers, got %d", expected, len(inspectionReads))
		}
	}
	for n := 0; n < maxInspectionReaders; n++ {
		release, ok := acquireInspectionClient(fmt.Sprintf("198.18.%d.%d", n/256, n%256), 4)
		if !ok {
			t.Fatal("independent client unexpectedly throttled", n)
		}
		reader, writer := io.Pipe()
		writers = append(writers, writer)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := readInspectionBody(ctx, reader, 8<<10, release)
			if err != context.Canceled {
				t.Error("unexpected stalled reader result", err)
			}
		}()
		if n == 95 {
			waitReaders(96)
			chunks, reserved, err := readInspectionBody(ctx, io.NopCloser(strings.NewReader("healthy")), 7, func() {})
			if err != nil || len(chunks) != 1 || string(chunks[0]) != "healthy" {
				t.Fatal("healthy body rejected behind 96 slow readers", err)
			}
			atomic.AddInt64(&inspectionBytes, -reserved)
		}
	}
	waitReaders(maxInspectionReaders)
	var released atomic.Bool
	_, _, err := readInspectionBody(ctx, io.NopCloser(strings.NewReader("overflow")), 8, func() { released.Store(true) })
	if err != errInspectionCapacity || !released.Load() {
		t.Fatal("reader ceiling did not reject and release admission", err)
	}
	if used := atomic.LoadInt64(&inspectionBytes); used > maxInspectionReaders*(8<<10) {
		t.Error("stalled readers exceeded the initial chunk budget", used)
	}
}

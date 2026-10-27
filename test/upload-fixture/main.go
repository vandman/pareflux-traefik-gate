// SPDX-License-Identifier: Apache-2.0
// FR: Confirme les octets reçus après une inspection partielle, sans les renvoyer ni les stocker.
// EN: Confirms bytes received after partial inspection without echoing or storing them.
// Called by: disposable Compose test; Calls: bounded HTTP body and SHA256.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var streamCalls = struct {
	sync.Mutex
	counts map[string]int
}{counts: make(map[string]int)}

// receive hashes the complete bounded request before returning a small response.
// FR: Une troncature du corps échoue au lieu de donner un succès prématuré.
// EN: Truncated bodies fail instead of reporting early success.
// Called by: HTTP server; Calls: MaxBytesReader, io.Copy, JSON.
func receive(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.URL.Path == "/v1/decisions/stream" {
		streamCalls.Lock()
		streamCalls.counts[r.Header.Get("X-Api-Key")]++
		streamCalls.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"new":[{"id":1,"scope":"country","value":"FR","type":"ban","duration":"1h"},{"id":2,"scope":"range","value":"192.0.2.0/24","type":"ban","duration":"1h"}]}`)
		return
	}
	if r.URL.Path == "/uploads/_stats" {
		streamCalls.Lock()
		defer streamCalls.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(streamCalls.counts)
		return
	}
	digest := sha256.New()
	n, err := io.Copy(digest, http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, "incomplete or oversized fixture request", 400)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/slow-response") {
		chunk := make([]byte, 16<<10)
		for n := 0; n < 1024; n++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"bytes": n, "sha256": hex.EncodeToString(digest.Sum(nil))})
}

// main starts a test-only receiver on the isolated Compose network.
// FR: Aucun port de ce récepteur n'est publié sur l'hôte.
// EN: No receiver port is published on the host.
// Called by: container entrypoint; Calls: bounded HTTP server.
func main() {
	server := &http.Server{Addr: ":8080", Handler: http.HandlerFunc(receive), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Fatal(server.ListenAndServe())
}

// SPDX-License-Identifier: Apache-2.0
// FR: Borne la mémoire d'inspection et libère le tampon après consommation du préfixe.
// EN: Bounds inspection memory and releases the single replay buffer after prefix consumption.
// Called by: appsecQuery, handleNextServeHTTP; Calls: HTTP contexts, bounded readers.
package pareflux_traefik_gate

import (
	"context"
	"errors"
	"fmt"
	"github.com/vandman/pareflux-traefik-gate/pkg/configuration"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxInspectionBytes int64 = 64 << 20

// FR: Borne les lecteurs indépendamment des octets, au-delà de l’ancien plafond de 64 lectures lentes.
// EN: Bounds readers independently of bytes, above the previous ceiling of 64 stalled reads.
// Called by: readInspectionBody; Calls: bounded worker channel.
const maxInspectionReaders = 1024

var inspectionBytes int64
var inspectionReads = make(chan struct{}, maxInspectionReaders)
var errInspectionUnavailable = errors.New("AppSec inspection failed")
var errBodyTooLarge = errors.New("AppSec body exceeds inspection limit")
var errInspectionCapacity = errors.New("AppSec memory budget exhausted")

// replayBody owns two native pipe writers sharing one inspection buffer.
// FR: Chaque écrivain libère sa référence dès consommation, indépendamment de la réponse applicative.
// EN: Each writer releases its reference on consumption, independently of the backend response.
// Called by: appsecQuery; Calls: native io.Pipe, atomic accounting.
type replayBody struct {
	original      io.ReadCloser
	backend       *io.PipeReader
	inspector     *io.PipeReader
	once          sync.Once
	remaining     int64
	reserved      int64
	releaseClient func()
}

// finish releases bytes only after both native writers have stopped accessing the buffer.
// FR: La réservation reste comptée tant qu'AppSec ou le backend lit encore le préfixe.
// EN: The reservation remains counted while AppSec or the backend still reads the prefix.
// Called by: writeInspectionPrefix; Calls: atomic accounting, client quota.
func (b *replayBody) finish() {
	if atomic.AddInt64(&b.remaining, -1) == 0 {
		atomic.AddInt64(&inspectionBytes, -b.reserved)
		b.releaseClient()
	}
}

// writeInspectionPrefix transfers the shared buffer without another body allocation.
// FR: La fermeture du lecteur débloque aussi un écrivain interrompu avant consommation complète.
// EN: Closing the reader also unblocks a writer interrupted before complete consumption.
// Called by: appsecQuery goroutines; Calls: native pipe Write and Close.
func writeInspectionPrefix(writer *io.PipeWriter, chunks [][]byte, limit int64, replay *replayBody) {
	for _, chunk := range chunks {
		if limit <= 0 {
			break
		}
		if int64(len(chunk)) > limit {
			chunk = chunk[:limit]
		}
		n, err := writer.Write(chunk)
		limit -= int64(n)
		if err != nil {
			break
		}
	}
	chunks = nil
	_ = writer.Close()
	replay.finish()
}

// Close interrupts native replay before closing a potentially slow original body.
// FR: Un client lent ne retient pas le préfixe pendant la fermeture du flux HTTP original.
// EN: A slow client cannot retain the prefix while the original HTTP stream closes.
// Called by: handleNextServeHTTP; Calls: native pipe Close, sync.Once.
func (b *replayBody) Close() error {
	var err error
	b.once.Do(func() { _ = b.backend.Close(); _ = b.inspector.Close(); err = b.original.Close() })
	return err
}

var inspectionClients = struct {
	sync.Mutex
	counts map[string]int
}{counts: make(map[string]int)}

// acquireInspectionClient bounds IPv4 clients and IPv6 /64 and /48 allocations.
// FR: La suppression à zéro et le plafond de 4096 clients bornent la mémoire du quota.
// EN: Removal at zero and a 4096-client ceiling bound quota memory.
// Called by: appsecQuery; Calls: netip, sync.Once.
func acquireInspectionClient(raw string, limit int) (func(), bool) {
	address, err := netip.ParseAddr(raw)
	if err != nil {
		return nil, false
	}
	address = address.Unmap()
	key := address.String()
	if address.Is6() {
		key = netip.PrefixFrom(address, 64).Masked().String()
	}
	if limit <= 0 {
		limit = 4
	}
	parent := ""
	if address.Is6() {
		parent = netip.PrefixFrom(address, 48).Masked().String()
	}
	inspectionClients.Lock()
	needed := 0
	if inspectionClients.counts[key] == 0 {
		needed++
	}
	if parent != "" && inspectionClients.counts[parent] == 0 {
		needed++
	}
	if inspectionClients.counts[key] >= limit || (parent != "" && inspectionClients.counts[parent] >= limit*4) || len(inspectionClients.counts)+needed > 4096 {
		inspectionClients.Unlock()
		return nil, false
	}
	inspectionClients.counts[key]++
	if parent != "" {
		inspectionClients.counts[parent]++
	}
	inspectionClients.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			inspectionClients.Lock()
			inspectionClients.counts[key]--
			if inspectionClients.counts[key] == 0 {
				delete(inspectionClients.counts, key)
			}
			if parent != "" {
				inspectionClients.counts[parent]--
				if inspectionClients.counts[parent] == 0 {
					delete(inspectionClients.counts, parent)
				}
			}
			inspectionClients.Unlock()
		})
	}, true
}

// reserveInspection rejects pressure immediately, rather than queuing unbounded requests.
// FR: Le compteur est commun à toutes les configurations du plugin.
// EN: The counter is shared across every plugin configuration.
// Called by: appsecQuery; Calls: atomic CAS.
func reserveInspection(n int64) bool {
	for {
		used := atomic.LoadInt64(&inspectionBytes)
		if n < 1 || n > maxInspectionBytes-used {
			return false
		}
		if atomic.CompareAndSwapInt64(&inspectionBytes, used, used+n) {
			return true
		}
	}
}

// partialAppsec explicitly marks configured upload and streaming exceptions.
// FR: Une inspection limitée aux en-têtes ne confirme jamais une inspection complète du corps.
// EN: Header-only inspection never confirms complete body inspection.
// Called by: appsecQuery; Calls: request-local proof.
func partialAppsec(r *http.Request) {
	if p, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof); ok {
		p.appsecPartial = true
	}
}

// appsecQuery applies one total deadline to body reading and the upstream inspection.
// FR: Les erreurs réseau sont distinctes d'un refus AppSec, toujours appliqué.
// EN: Network failures are distinct from AppSec denials, which are always enforced.
// Called by: handleNextServeHTTP, tests; Calls: bounded body reservation and AppSec HTTP.
func appsecQuery(b *Bouncer, ip string, r *http.Request) error {
	if _, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof); !ok {
		*r = *r.WithContext(context.WithValue(r.Context(), gateContextKey{}, &gateRequestProof{gate: b}))
	}
	budget := b.appsecTimeout
	if budget <= 0 {
		budget = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), budget)
	defer cancel()
	target := url.URL{Scheme: b.appsecScheme, Host: b.appsecHost, Path: b.appsecPath}
	inspectionAccepted := false
	var inspectedLength int64
	var inspectorBody io.Reader
	if shouldInspectAppsecBody(b, r) {
		releaseClient, allowed := acquireInspectionClient(ip, b.appsecMaxConcurrentPerClient)
		if !allowed {
			return errInspectionCapacity
		}
		size := b.appsecBodyLimit + 1
		if r.ContentLength > 0 && r.ContentLength < size {
			size = r.ContentLength
		}
		original := r.Body
		chunks, reserved, err := readInspectionBody(ctx, original, size, releaseClient)
		if err != nil {
			return err
		}
		var n int64
		for _, chunk := range chunks {
			n += int64(len(chunk))
		}
		if n > b.appsecBodyLimit && !b.partialBodyAllowed(r) {
			atomic.AddInt64(&inspectionBytes, -reserved)
			releaseClient()
			return errBodyTooLarge
		}
		backend, backendWriter := io.Pipe()
		inspector, inspectorWriter := io.Pipe()
		replay := &replayBody{original: original, backend: backend, inspector: inspector, reserved: reserved, remaining: 2, releaseClient: releaseClient}
		r.Body = io.NopCloser(io.MultiReader(backend, original))
		inspectedLength = n
		if inspectedLength > b.appsecBodyLimit {
			inspectedLength = b.appsecBodyLimit
		}
		inspectorBody = inspector
		go writeInspectionPrefix(backendWriter, chunks, n, replay)
		go writeInspectionPrefix(inspectorWriter, chunks, inspectedLength, replay)
		defer inspector.Close()
		defer func() {
			if !inspectionAccepted {
				_ = backend.Close()
			}
		}()
		if proof, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof); ok {
			proof.replay = replay
		}
		if n > b.appsecBodyLimit {
			partialAppsec(r)
		}

	} else if r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
		partialAppsec(r)
	}
	method := http.MethodGet
	if inspectorBody != nil {
		method = http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), inspectorBody)
	if err != nil {
		return err
	}
	if inspectorBody != nil {
		request.ContentLength = inspectedLength
	}
	for k, values := range r.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Authorization", "Cookie", "Proxy-Authorization", "Connection", "Upgrade", "Transfer-Encoding", "Content-Length":
			continue
		}
		for _, v := range values {
			request.Header.Add(k, v)
		}
	}
	request.Header.Set(crowdsecAppsecHeader, b.appsecKey)
	request.Header.Set(crowdsecAppsecIPHeader, ip)
	request.Header.Set(crowdsecAppsecVerbHeader, r.Method)
	request.Header.Set(crowdsecAppsecHostHeader, r.Host)
	request.Header.Set(crowdsecAppsecURIHeader, r.URL.String())
	request.Header.Set(crowdsecAppsecUserAgent, r.Header.Get("User-Agent"))
	response, err := b.httpAppsecClient.Do(request)
	if err != nil {
		b.log.Warn("AppSec inspection unavailable")
		if r.Context().Err() != nil {
			return r.Context().Err()
		}
		var connectionError *net.OpError
		if ctx.Err() == nil && errors.As(err, &connectionError) && connectionError.Op == "dial" && (!b.appsecUnreachableBlock || b.appsecWithinGrace()) {
			inspectionAccepted = true
			return nil
		}
		return fmt.Errorf("%w: %v", errInspectionUnavailable, err)
	}
	defer response.Body.Close()
	responseSize, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, maxAppsecResponseBytes+1))
	if readErr != nil || responseSize > maxAppsecResponseBytes {
		return fmt.Errorf("%w: incomplete or oversized response", errInspectionUnavailable)
	}
	if response.StatusCode >= 500 {
		if (response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504) && (!b.appsecFailureBlock || b.appsecWithinGrace()) {
			inspectionAccepted = true
			return nil
		}
		return fmt.Errorf("%w: HTTP %d", errInspectionUnavailable, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("AppSec denial: %d", response.StatusCode)
	}
	if b.engine != nil {
		atomic.StoreInt64(&b.engine.lastAppsecSuccess, time.Now().Unix())
		atomic.StoreInt64(&b.engine.appsecFailures, 0)
	}
	if p, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof); ok {
		p.appsecConfirmed = !p.appsecPartial
	}
	inspectionAccepted = true
	return nil
}

// appsecWithinGrace permits only short upstream outages after a successful inspection.
// FR: Une installation jamais vérifiée reste bloquante ; les refus malveillants n'appellent pas ce chemin.
// EN: An unverified installation stays fail-closed; malicious denials never call this path.
// Called by: appsecQuery transport/5xx errors; Calls: atomic health evidence.
func (b *Bouncer) appsecWithinGrace() bool {
	if b.engine == nil {
		return false
	}
	failures := atomic.AddInt64(&b.engine.appsecFailures, 1)
	last := atomic.LoadInt64(&b.engine.lastAppsecSuccess)
	grace := last > 0 && (time.Now().Unix()-last <= b.failureGraceSeconds || b.updateMaxFailure == -1 || failures <= b.updateMaxFailure)
	b.log.Warn("AppSec degraded", "grace", grace, "failures", failures)
	return grace
}

// shouldInspectAppsecBody permits protocol exemptions only for valid streaming requests.
// FR: Les types MIME seuls ne désactivent pas l'inspection ; une exception d'upload est liée à la route.
// EN: MIME types alone never disable inspection; upload exceptions belong to the route.
// Called by: appsecQuery; Calls: tokenized HTTP protocol validation.
func shouldInspectAppsecBody(b *Bouncer, r *http.Request) bool {
	if b.appsecBodyLimit <= 0 || r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return false
	}

	return true
}

// readInspectionBody bounds workers and progressively reserves shared chunks.
// FR: Un canal fermé assure la visibilité des résultats sans les conversions de structures Yaegi.
// EN: A closed channel makes results visible without Yaegi struct-channel conversions.
// Called by: appsecQuery; Calls: one bounded reader and bounded cancellation cleanup.
func readInspectionBody(ctx context.Context, body io.ReadCloser, size int64, releaseClient func()) ([][]byte, int64, error) {
	select {
	case inspectionReads <- struct{}{}:
	default:
		releaseClient()
		return nil, 0, errInspectionCapacity
	}
	done := make(chan struct{})
	var chunks [][]byte
	var reserved int64
	var readErr error
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	go func() {
		defer func() { <-inspectionReads; close(done) }()
		remaining := size
		for remaining > 0 {
			amount := int64(8 << 10)
			if remaining < amount {
				amount = remaining
			}
			if !reserveInspection(amount) {
				readErr = errInspectionCapacity
				return
			}
			reserved += amount
			chunk := make([]byte, amount)
			n, err := io.ReadFull(body, chunk)
			if n > 0 {
				chunks = append(chunks, chunk[:n])
				remaining -= int64(n)
			} else {
				chunk = nil
				atomic.AddInt64(&inspectionBytes, -amount)
				reserved -= amount
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			}
			if err != nil {
				readErr = err
				return
			}
			if ctx.Err() != nil {
				readErr = ctx.Err()
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		go func() { <-done; chunks = nil; atomic.AddInt64(&inspectionBytes, -reserved); releaseClient() }()
		return nil, 0, ctx.Err()
	case <-done:
		stop()
		if ctx.Err() != nil {
			readErr = ctx.Err()
		}
		if readErr != nil {
			chunks = nil
			atomic.AddInt64(&inspectionBytes, -reserved)
			releaseClient()
			return nil, 0, readErr
		}
		return chunks, reserved, nil
	}
}

// releaseInspectionBody closes the original stream and reservation after downstream processing.
// FR: Le corps HTTP utilise des lecteurs natifs, évitant les adaptateurs de corps Yaegi.
// EN: The HTTP body uses native readers, avoiding Yaegi body interface adapters.
// Called by: handleNextServeHTTP and direct inspection tests; Calls: replayBody.Close.
func releaseInspectionBody(r *http.Request) {
	if p, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof); ok && p.replay != nil {
		_ = p.replay.Close()
	}
}

// compiledBodyException binds a precompiled path to one exact host and explicit upload methods.
// FR: Les expressions sont compilées une seule fois par configuration de handler.
// EN: Expressions are compiled once per handler configuration.
// Called by: New; Calls: regexp.
type compiledBodyException struct {
	host    string
	path    *regexp.Regexp
	methods []string
}

// compileBodyExceptions prepares explicit route exemptions before handling requests.
// FR: Une expression invalide empêche l'activation de la configuration.
// EN: An invalid expression prevents configuration activation.
// Called by: New; Calls: regexp.Compile.
func compileBodyExceptions(rules []configuration.AppsecBodyException) ([]compiledBodyException, error) {
	out := make([]compiledBodyException, 0, len(rules))
	for _, rule := range rules {
		pattern, err := regexp.Compile(rule.PathRegexp)
		if err != nil {
			return nil, err
		}
		out = append(out, compiledBodyException{host: strings.ToLower(strings.TrimSuffix(rule.Host, ".")), path: pattern, methods: rule.Methods})
	}
	return out, nil
}

// partialBodyAllowed rejects ambiguous paths and keeps upload exceptions out of sensitive routes.
// FR: Une exception de corps n'exempte jamais l'adresse source ou les métadonnées AppSec.
// EN: Body exceptions never exempt source addresses or AppSec metadata.
// Called by: appsecQuery; Calls: precompiled expressions.
func (b *Bouncer) partialBodyAllowed(r *http.Request) bool {
	if b.appsecBodyLimitAction == "partial" {
		return true
	}
	clean := path.Clean(r.URL.Path)
	if clean != strings.TrimSuffix(r.URL.Path, "/") && !(clean == "/" && r.URL.Path == "/") {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, rule := range b.appsecBodyExceptions {
		if host != rule.host || !rule.path.MatchString(r.URL.Path) {
			continue
		}
		for _, method := range rule.methods {
			if r.Method == method {
				return true
			}
		}
	}
	return false
}

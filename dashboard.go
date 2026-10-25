// SPDX-License-Identifier: Apache-2.0
// FR: Ajoute un lien au dashboard natif sans proxy externe ni modification des contrôles de sécurité.
// EN: Adds a native dashboard link without an external proxy or changes to security enforcement.
// Called by: dedicated plugin middleware instance; Calls: downstream Traefik dashboard.
package pareflux_traefik_gate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const dashboardBodyLimit = 2 << 20

// dashboardMenu is opt-in and must follow the route's authentication middleware.
// FR: Cette instance distincte ne remplace jamais le middleware de protection existant.
// EN: This separate instance never replaces the existing protection middleware.
// Called by: New; Calls: dashboardResponse.
type dashboardMenu struct {
	next         http.Handler
	host         string
	link         string
	healthURL    string
	registration *dashboardRegistration
}

// newDashboardMenu validates an exact host and a safe relative or HTTPS target.
// FR: Refuse les liens actifs, les URL ambiguës et les identifiants embarqués.
// EN: Rejects active links, ambiguous URLs and embedded credentials.
// Called by: New; Calls: net/url.
func newDashboardMenu(next http.Handler, host, link string) (http.Handler, error) {
	parsed, err := url.Parse(link)
	if err != nil || link == "" || strings.ContainsAny(link, "\\\r\n") || parsed.User != nil || parsed.Opaque != "" || !(parsed.Scheme == "https" && parsed.Hostname() != "" || parsed.Scheme == "" && parsed.Host == "" && strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//")) {
		return nil, errors.New("invalid dashboard link URL")
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || strings.ContainsAny(host, "/:@?#* \\\r\n") {
		return nil, errors.New("dashboard link requires an exact hostname")
	}
	return &dashboardMenu{next: next, host: host, link: link}, nil
}

// ServeHTTP decorates only successful entry HTML and keeps other responses unchanged.
// FR: La redirection reste locale ; les refus SSO, blocages, actifs et API sont préservés.
// EN: Redirects remain local; SSO refusals, bans, assets and API responses are preserved.
// Called by: Traefik; Calls: next.ServeHTTP and dashboardResponse.finish.
func (m *dashboardMenu) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.registration != nil && r.Method == http.MethodPost && r.URL.Path == "/.well-known/pareflux" {
		m.register(w, r)
		return
	}

	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if r.Method != http.MethodGet || strings.ToLower(strings.TrimSuffix(host, ".")) != m.host || r.Header.Get("Upgrade") != "" {
		m.next.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/dashboard/pareflux-status" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(m.status(r))
		return
	}

	if r.URL.Path == "/dashboard" {
		target := "/dashboard/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	if r.URL.Path != "/dashboard/" {
		m.next.ServeHTTP(w, r)
		return
	}
	link := m.link
	warning := false
	if m.registration != nil {
		m.registration.mu.Lock()
		if m.registration.link != "" {
			link = m.registration.link
		}
		m.registration.mu.Unlock()
		warning = !m.ready(r.Context())
	}
	if link == "" {
		m.next.ServeHTTP(w, r)
		return
	}
	response := &dashboardResponse{out: w, ui: m.status(r)}
	m.next.ServeHTTP(response, r)
	response.finish(link, warning, dashboardWarning(r.Header.Get("Accept-Language")))
}

// dashboardResponse bounds buffering and falls back to the original response for streaming or large bodies.
// FR: Le tampon ne concerne que le HTML 200 non compressé du dashboard.
// EN: Buffering covers only uncompressed dashboard HTML with status 200.
// Called by: dashboardMenu; Calls: http.ResponseWriter.
type dashboardResponse struct {
	ui      dashboardStatus
	out     http.ResponseWriter
	body    bytes.Buffer
	status  int
	capture bool
}

// Header exposes the native response headers without copying authentication metadata.
// FR: Les en-têtes de sécurité restent ceux du service natif.
// EN: Security headers remain those of the native service.
// Called by: downstream handler; Calls: ResponseWriter.Header.
func (w *dashboardResponse) Header() http.Header { return w.out.Header() }

// WriteHeader selects eligible HTML, immediately forwarding errors and redirects.
// FR: Aucune page de refus ne reçoit le lien.
// EN: Refusal pages never receive the link.
// Called by: downstream handler and Write; Calls: ResponseWriter.WriteHeader.
func (w *dashboardResponse) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.out.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	encoding := w.Header().Get("Content-Encoding")
	w.capture = status == http.StatusOK && strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/html") && (encoding == "" || encoding == "identity")
	if !w.capture {
		w.out.WriteHeader(status)
	}
}

// Write buffers eligible HTML up to a fixed cap, then streams the original bytes.
// FR: Un dépassement restitue aussi le préfixe déjà tamponné sans perte de données.
// EN: Overflow also releases the already buffered prefix without losing data.
// Called by: downstream handler; Calls: release and ResponseWriter.Write.
func (w *dashboardResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.capture {
		if w.body.Len()+len(data) <= dashboardBodyLimit {
			return w.body.Write(data)
		}
		w.release()
	}
	return w.out.Write(data)
}

// release forwards the original buffered response without transforming it.
// FR: Préserve intégralement le corps et ses validateurs lors d'un repli.
// EN: Preserves the complete body and validators on fallback.
// Called by: Write, Flush and finish; Calls: ResponseWriter.
func (w *dashboardResponse) release() {
	if !w.capture {
		return
	}
	w.capture = false
	w.out.WriteHeader(w.status)
	w.out.Write(w.body.Bytes())
	w.body.Reset()
}

// Flush preserves streaming behavior by disabling transformation for this response.
// FR: Ne retient pas une réponse que le service veut envoyer immédiatement.
// EN: Does not retain a response the service wants to send immediately.
// Called by: downstream streaming handlers; Calls: release and Flusher.
func (w *dashboardResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.release()
	if f, ok := w.out.(http.Flusher); ok {
		f.Flush()
	}
}

// finish inserts one escaped link and discards validators only when the HTML changes.
// FR: Le HTML reste natif ; aucun appel à PareFlux Control n'est effectué.
// EN: HTML remains native; no PareFlux Control request is made.
// Called by: dashboardMenu; Calls: bytes.Replace and ResponseWriter.
func (w *dashboardResponse) finish(link string, warning bool, tooltip string) {
	if !w.capture {
		return
	}
	body := w.body.Bytes()
	if !bytes.Contains(body, []byte("</body>")) || bytes.Contains(body, []byte(`id="pareflux-dashboard-menu"`)) {
		w.release()
		return
	}
	label := "PareFlux ↗"
	state := "available"
	extra := ""
	if warning {
		label = "PareFlux ⚠"
		state = "unavailable"
		extra = ` title="` + html.EscapeString(tooltip) + `"`
	}
	menu := `<nav id="pareflux-dashboard-menu" aria-label="PareFlux" data-state="` + state + `" style="position:fixed;bottom:16px;right:16px;z-index:6000"><a href="` + html.EscapeString(link) + `"` + extra + ` style="display:block;padding:12px 22px;border:1px solid #32d8d2;border-radius:6px;background:#10272d;color:#fff;font:600 15px system-ui;text-decoration:none;box-shadow:0 2px 10px #0005">` + label + `</a></nav>`
	body = bytes.Replace(body, []byte("</body>"), []byte(menu+dashboardBootstrap(w.ui)+"</body>"), 1)
	for _, header := range []string{"Content-Length", "ETag", "Last-Modified", "Content-MD5", "Digest"} {
		w.Header().Del(header)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.out.WriteHeader(w.status)
	w.out.Write(body)
	w.capture = false
}

// dashboardRegistration shares a short-lived registration across router middleware instances.
// FR: Le lien annoncé expire si Control ne confirme plus sa présence.
// EN: The announced link expires when Control stops confirming its presence.
// Called by: dashboard integration; Calls: sync.Mutex.
type dashboardRegistration struct {
	healthURL string
	mu        sync.Mutex
	link      string
	expires   time.Time
	healthAt  time.Time
	healthy   bool
}

// FR: Yaegi charge le même registre pour les routeurs privé et public du plugin.
// EN: Yaegi loads the same registry for the plugin's private and public routers.
// Called by: newDashboardIntegration; Calls: none.
var dashboardRegistrations = struct {
	sync.Mutex
	states map[string]*dashboardRegistration
}{states: map[string]*dashboardRegistration{}}

// newDashboardIntegration enables private mutual URL discovery without a configured Control URL.
// FR: Seul le nom du dashboard local est configuré ; Control annonce sa propre URL.
// EN: Only the local dashboard hostname is configured; Control announces its own URL.
// Called by: New; Calls: newDashboardMenu.
func newDashboardIntegration(next http.Handler, host, link, healthURL string) (http.Handler, error) {
	checkedLink := link
	if checkedLink == "" {
		checkedLink = "/"
	}
	handler, err := newDashboardMenu(next, host, checkedLink)
	if err != nil {
		return nil, err
	}
	menu := handler.(*dashboardMenu)
	menu.link = link
	if healthURL != "" {
		health, err := url.Parse(healthURL)
		if err != nil || (health.Scheme != "http" && health.Scheme != "https") || health.Hostname() == "" || health.User != nil || health.Fragment != "" || health.RawQuery != "" {
			return nil, errors.New("invalid dashboard health URL")
		}
	}
	menu.healthURL = healthURL
	dashboardRegistrations.Lock()
	state := dashboardRegistrations.states[menu.host]
	if state == nil {
		state = &dashboardRegistration{}
		dashboardRegistrations.states[menu.host] = state
	}
	dashboardRegistrations.Unlock()
	menu.registration = state
	state.mu.Lock()
	if state.healthURL == "" {
		state.healthURL = healthURL
	}
	state.mu.Unlock()
	return menu, nil
}

// register accepts only bounded private announcements and returns this ingress's dashboard URL.
// FR: Ce chemin POST doit être exposé uniquement au Control sur un entrypoint privé.
// EN: This POST path must be exposed only to Control on a private entrypoint.
// Called by: private middleware route; Calls: JSON and URL validation.
func (m *dashboardMenu) register(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "Forbidden", 403)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	if err != nil || len(body) > 4096 {
		http.Error(w, "Invalid announcement", 400)
		return
	}
	var announcement struct {
		ControlURL string `json:"control_url"`
		Nonce      string `json:"nonce"`
		HealthPath string `json:"health_path"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&announcement) != nil || decoder.Decode(new(interface{})) != io.EOF || len(announcement.Nonce) < 16 || len(announcement.Nonce) > 128 {
		http.Error(w, "Invalid announcement", 400)
		return
	}
	target, err := url.Parse(announcement.ControlURL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || strings.ContainsAny(announcement.ControlURL, "\\\r\n") {
		http.Error(w, "Invalid control URL", 400)
		return
	}
	healthURL := m.healthURL
	if announcement.HealthPath != "" && healthURL != "" {
		path, err := url.Parse(announcement.HealthPath)
		if err != nil || !strings.HasPrefix(announcement.HealthPath, "/") || path.Host != "" || path.Scheme != "" || path.RawQuery != "" || path.Fragment != "" || strings.ContainsAny(announcement.HealthPath, "\\\r\n") {
			http.Error(w, "Invalid health path", 400)
			return
		}
		health, _ := url.Parse(healthURL)
		health.Path = path.Path
		health.RawPath = path.RawPath
		healthURL = health.String()
	}

	m.registration.mu.Lock()
	m.registration.link = announcement.ControlURL
	if m.registration.healthURL != healthURL {
		m.registration.healthURL = healthURL
		m.registration.healthAt = time.Time{}
	}
	m.registration.expires = time.Now().Add(90 * time.Second)
	m.registration.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{"protocol": "pareflux.ingress.v1", "name": "Traefik", "dashboard_url": "https://" + m.host + "/dashboard/", "control_url": announcement.ControlURL, "nonce": announcement.Nonce, "expires_in": 90, "control_reachable": m.ready(r.Context())})
}

// ready verifies a bounded native readiness response, not an SSO login page.
// FR: La vérification est mutualisée quinze secondes et n'envoie aucun identifiant.
// EN: Checks are shared for fifteen seconds and send no credentials.
// Called by: ServeHTTP and register; Calls: private Control readiness endpoint.
func (m *dashboardMenu) ready(ctx context.Context) bool {
	state := m.registration
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.healthURL == "" {
		return time.Now().Before(state.expires)
	}
	if !state.healthAt.IsZero() && time.Since(state.healthAt) < 15*time.Second {
		return state.healthy
	}
	state.healthAt = time.Now()
	state.healthy = false
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, state.healthURL, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 750 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2049))
	if err != nil || response.StatusCode != 200 || len(data) > 2048 {
		return false
	}
	var health struct {
		Status string `json:"status"`
	}
	state.healthy = json.Unmarshal(data, &health) == nil && health.Status == "ready"
	return state.healthy
}

// dashboardWarning localizes the availability tooltip without exposing internal addresses.
// FR: Les 24 langues officielles de l'UE et le coréen partagent le même sens.
// EN: The 24 official EU languages and Korean share the same warning meaning.
// Called by: dashboardMenu.ServeHTTP; Calls: none.
func dashboardWarning(language string) string {
	code := strings.ToLower(strings.TrimSpace(strings.Split(strings.Split(language, ",")[0], ";")[0]))
	if len(code) > 2 {
		code = code[:2]
	}
	values := map[string]string{
		"fr": "PareFlux ne répond pas ou n’est pas prêt. Réessayez dans quelques instants.",
		"en": "PareFlux is unreachable or not ready. Please try again shortly.",
		"bg": "PareFlux е недостъпен или не е готов. Опитайте отново след малко.",
		"hr": "PareFlux nije dostupan ili nije spreman. Pokušajte ponovno uskoro.",
		"cs": "PareFlux není dostupný nebo připravený. Zkuste to za chvíli znovu.",
		"da": "PareFlux kan ikke nås eller er ikke klar. Prøv igen om lidt.",
		"nl": "PareFlux is niet bereikbaar of niet gereed. Probeer het zo opnieuw.",
		"et": "PareFlux pole kättesaadav või valmis. Proovige mõne hetke pärast uuesti.",
		"fi": "PareFlux ei ole tavoitettavissa tai valmis. Yritä pian uudelleen.",
		"de": "PareFlux ist nicht erreichbar oder nicht bereit. Bitte versuchen Sie es gleich erneut.",
		"el": "Το PareFlux δεν είναι προσβάσιμο ή έτοιμο. Δοκιμάστε ξανά σε λίγο.",
		"hu": "A PareFlux nem érhető el vagy nem áll készen. Próbálja újra hamarosan.",
		"ga": "Níl PareFlux insroichte nó réidh. Bain triail eile as ar ball.",
		"it": "PareFlux non è raggiungibile o pronto. Riprova tra poco.",
		"lv": "PareFlux nav sasniedzams vai gatavs. Mēģiniet vēlreiz pēc brīža.",
		"lt": "PareFlux nepasiekiamas arba neparengtas. Netrukus bandykite dar kartą.",
		"mt": "PareFlux mhuwiex aċċessibbli jew lest. Erġa’ pprova dalwaqt.",
		"pl": "PareFlux jest niedostępny lub niegotowy. Spróbuj ponownie za chwilę.",
		"pt": "O PareFlux está inacessível ou não está pronto. Tente novamente em breve.",
		"ro": "PareFlux nu este accesibil sau pregătit. Încercați din nou în curând.",
		"sk": "PareFlux nie je dostupný alebo pripravený. Skúste to o chvíľu znova.",
		"sl": "PareFlux ni dosegljiv ali pripravljen. Poskusite znova čez nekaj trenutkov.",
		"es": "PareFlux no está disponible o preparado. Vuelve a intentarlo en unos instantes.",
		"sv": "PareFlux kan inte nås eller är inte redo. Försök igen om en stund.",
		"ko": "PareFlux에 연결할 수 없거나 서비스가 준비되지 않았습니다. 잠시 후 다시 시도하세요.",
	}
	if value, ok := values[code]; ok {
		return value
	}
	return values["en"]
}

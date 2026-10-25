// SPDX-License-Identifier: Apache-2.0
// FR: Intègre navigation et état vérifié aux composants natifs du dashboard Traefik.
// EN: Integrates navigation and verified status into the native Traefik dashboard.
// Called by: dashboard middleware; Calls: request-local security proof and native DOM.
package pareflux_traefik_gate

import (
	"encoding/json"
	"github.com/vandman/pareflux-traefik-gate/pkg/configuration"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// gateContextKey identifies evidence that only the security middleware can attach.
// FR: Un en-tête envoyé par le navigateur ne peut pas fabriquer cette preuve.
// EN: A browser-supplied header cannot fabricate this evidence.
// Called by: Bouncer.ServeHTTP and dashboard status; Calls: context values.
type gateContextKey struct{}

// gateRequestProof confirms the actual middleware chain and this request's AppSec success.
// FR: Une console joignable seule ne suffit jamais à afficher la protection en vert.
// EN: Console reachability alone can never make protection green.
// Called by: security middleware; Calls: none.
type gateRequestProof struct {
	gate            *Bouncer
	appsecConfirmed bool
}

// dashboardStatus contains only public display facts, never security keys or addresses.
// FR: Les libellés et URL publiques sont bornés et échappés avant affichage.
// EN: Labels and public URLs are bounded and escaped before display.
// Called by: native dashboard endpoint and bootstrap; Calls: none.
type dashboardStatus struct {
	URL       string   `json:"url"`
	Connected bool     `json:"connected"`
	Protected bool     `json:"protected"`
	Labels    []string `json:"labels"`
	Warning   string   `json:"warning"`
}

// status checks the console and actual request protection independently.
// FR: Le flux LAPI doit avoir réussi récemment ; AppSec activé doit avoir validé la requête.
// EN: LAPI streaming must have succeeded recently; enabled AppSec must validate this request.
// Called by: dashboard middleware; Calls: readiness and atomic health evidence.
func (m *dashboardMenu) status(r *http.Request) dashboardStatus {
	value := dashboardStatus{URL: m.link, Connected: true, Labels: dashboardLabels(r.Header.Get("Accept-Language")), Warning: dashboardWarning(r.Header.Get("Accept-Language"))}
	if m.registration != nil {
		m.registration.mu.Lock()
		if m.registration.link != "" {
			value.URL = m.registration.link
		}
		m.registration.mu.Unlock()
		value.Connected = m.ready(r.Context())
	}
	proof, ok := r.Context().Value(gateContextKey{}).(*gateRequestProof)
	if !ok || proof.gate == nil || !proof.gate.enabled {
		return value
	}
	gate := proof.gate
	engine := false
	switch gate.crowdsecMode {
	case configuration.StreamMode, configuration.AloneMode:
		age := time.Now().Unix() - atomic.LoadInt64(&lastStreamSuccess)
		limit := gate.updateInterval * 3
		if limit < 30 {
			limit = 30
		}
		engine = crowdsecStreamIsHealthy() && atomic.LoadInt64(&lastStreamSuccess) > 0 && age <= limit
	case configuration.LiveMode:
		limit := gate.defaultDecisionTimeout
		if limit < 30 {
			limit = 30
		}
		engine = atomic.LoadInt64(&lastLiveSuccess) > 0 && time.Now().Unix()-atomic.LoadInt64(&lastLiveSuccess) <= limit
	case configuration.AppsecMode, configuration.NoneMode:
		engine = gate.appsecEnabled && proof.appsecConfirmed
	}
	value.Protected = engine && (!gate.appsecEnabled || proof.appsecConfirmed)
	return value
}

// dashboardLabels localizes independent connectivity and protection states.
// FR: Les couleurs restent accompagnées d'un texte dans chaque langue disponible.
// EN: Colors are accompanied by text in every available language.
// Called by: status; Calls: none.
func dashboardLabels(language string) []string {
	code := strings.ToLower(strings.TrimSpace(strings.Split(strings.Split(language, ",")[0], ";")[0]))
	if len(code) > 2 {
		code = code[:2]
	}
	labels := map[string][]string{
		"fr": {"Connecté", "Déconnecté", "Protégé", "Non confirmé"}, "en": {"Connected", "Disconnected", "Protected", "Unconfirmed"},
		"bg": {"Свързан", "Няма връзка", "Защитен", "Непотвърдено"}, "hr": {"Povezano", "Nije povezano", "Zaštićeno", "Nepotvrđeno"},
		"cs": {"Připojeno", "Odpojeno", "Chráněno", "Nepotvrzeno"}, "da": {"Forbundet", "Afbrudt", "Beskyttet", "Ubekræftet"},
		"nl": {"Verbonden", "Verbroken", "Beschermd", "Onbevestigd"}, "et": {"Ühendatud", "Ühendus puudub", "Kaitstud", "Kinnitamata"},
		"fi": {"Yhdistetty", "Ei yhteyttä", "Suojattu", "Vahvistamaton"}, "de": {"Verbunden", "Getrennt", "Geschützt", "Nicht bestätigt"},
		"el": {"Συνδεδεμένο", "Αποσυνδεδεμένο", "Προστατευμένο", "Μη επιβεβαιωμένο"}, "hu": {"Csatlakoztatva", "Nincs kapcsolat", "Védett", "Nem megerősített"},
		"ga": {"Ceangailte", "Dícheangailte", "Cosanta", "Gan deimhniú"}, "it": {"Connesso", "Disconnesso", "Protetto", "Non confermato"},
		"lv": {"Savienots", "Atvienots", "Aizsargāts", "Neapstiprināts"}, "lt": {"Prijungta", "Atjungta", "Apsaugota", "Nepatvirtinta"},
		"mt": {"Konness", "Skonness", "Protett", "Mhux ikkonfermat"}, "pl": {"Połączono", "Rozłączono", "Chronione", "Niepotwierdzone"},
		"pt": {"Ligado", "Desligado", "Protegido", "Não confirmado"}, "ro": {"Conectat", "Deconectat", "Protejat", "Neconfirmat"},
		"sk": {"Pripojené", "Odpojené", "Chránené", "Nepotvrdené"}, "sl": {"Povezano", "Prekinjeno", "Zaščiteno", "Nepotrjeno"},
		"es": {"Conectado", "Desconectado", "Protegido", "Sin confirmar"}, "sv": {"Ansluten", "Frånkopplad", "Skyddad", "Obekräftad"},
		"ko": {"연결됨", "연결 끊김", "보호됨", "확인되지 않음"},
	}
	if value, ok := labels[code]; ok {
		return value
	}
	return labels["en"]
}

// dashboardBootstrap decorates native semantic elements and polls only the SSO-protected status route.
// FR: L'observateur est idempotent, conserve les éléments React et évite une boucle de rendu.
// EN: The observer is idempotent, preserves React elements and avoids a rendering loop.
// Called by: dashboardResponse.finish; Calls: native browser DOM and same-origin fetch.
func dashboardBootstrap(status dashboardStatus) string {
	data, _ := json.Marshal(status)
	return `<script id="pareflux-dashboard-bootstrap">(()=>{let state=` + string(data) + `;let scheduled=false;
const setText=(e,value)=>{if(e.textContent!==value)e.textContent=value;};
const fallback=()=>document.getElementById('pareflux-dashboard-menu');
function render(){scheduled=false;const root=document.getElementById('root');if(!root)return;
 const catalog=[...root.querySelectorAll('a')].find(a=>a.textContent.trim()==='Plugins');
 if(catalog){const parent=catalog.parentElement;let heading=document.getElementById('pareflux-plugin-heading');if(!heading){const http=root.querySelector('a[href$="/http/routers"]');const template=http?.parentElement.querySelector('span');heading=template?template.cloneNode(false):document.createElement('span');heading.id='pareflux-plugin-heading';heading.textContent='PLUGINS';parent.insertBefore(heading,catalog);}
 let link=document.getElementById('pareflux-plugin-link');if(!link){link=catalog.cloneNode(true);link.id='pareflux-plugin-link';link.removeAttribute('target');link.removeAttribute('rel');const label=link.lastElementChild;if(label)label.textContent='PareFlux';else link.textContent='PareFlux';parent.append(link);}
 if(link.getAttribute('href')!==state.url)link.setAttribute('href',state.url);link.title=state.connected?'PareFlux':state.warning;link.dataset.state=state.connected?'available':'unavailable';if(link.lastElementChild)setText(link.lastElementChild,state.connected?'PareFlux':'PareFlux ⚠');if(fallback())fallback().style.display='none';
 }else if(fallback())fallback().style.display='';
 const heading=[...root.querySelectorAll('h1,h2,h3,h4')].find(h=>h.textContent.trim()==='Features');const grid=heading?.parentElement.nextElementSibling;
 if(grid?.firstElementChild){let card=document.getElementById('pareflux-feature');if(!card){const template=grid.firstElementChild;card=template.cloneNode(false);card.id='pareflux-feature';const inner=template.firstElementChild?.cloneNode(false)||document.createElement('div');inner.style.minHeight='120px';const title=template.querySelector('span')?.cloneNode(false)||document.createElement('span');title.textContent='PareFlux / CrowdSec';title.style.cssText='font-size:12px;letter-spacing:1px;white-space:nowrap';inner.append(title);const badges=document.createElement('div');badges.style.cssText='display:flex;flex-direction:column;align-items:center;gap:6px';for(const kind of ['connected','protected']){const badge=document.createElement('span');badge.id='pareflux-feature-'+kind;badge.style.cssText='padding:5px 12px;border-radius:4px;font:600 14px system-ui';badges.append(badge);}inner.append(badges);card.append(inner);grid.append(card);}
 card.dataset.state=state.connected&&state.protected?'healthy':'degraded';const connected=document.getElementById('pareflux-feature-connected'),protectedBadge=document.getElementById('pareflux-feature-protected');for(const [badge,ok,label]of [[connected,state.connected,state.labels[state.connected?0:1]],[protectedBadge,state.protected,state.labels[state.protected?2:3]]]){setText(badge,label);badge.dataset.state=ok?'healthy':'degraded';badge.style.backgroundColor=ok?'#145b3a':'#7d2039';badge.style.color=ok?'#c1f9d4':'#ffe5ed';badge.title=ok?label:state.connected?'CrowdSec — '+state.labels[3]:state.warning;}
 }
}
const schedule=()=>{if(!scheduled){scheduled=true;requestAnimationFrame(render);}};new MutationObserver(schedule).observe(document.getElementById('root')||document.body,{childList:true,subtree:true});schedule();
async function refresh(){try{const response=await fetch('/dashboard/pareflux-status',{credentials:'same-origin',cache:'no-store'});if(!response.ok)throw Error();const value=await response.json();if(typeof value.url==='string'&&Array.isArray(value.labels))state=value;else throw Error();}catch{state={...state,connected:false,protected:false};}schedule();}
refresh();setInterval(refresh,15000);})();</script>`
}

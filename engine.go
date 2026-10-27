// SPDX-License-Identifier: Apache-2.0
// FR: Partage les synchronisations entre routes identiques et libère les workers au dernier départ.
// EN: Shares synchronization across matching routes and releases workers after the last user.
// Called by: New, dashboard; Calls: context, stream synchronizer, reference registry.
package pareflux_traefik_gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/vandman/pareflux-traefik-gate/pkg/configuration"
)

// engineState owns only one configuration's runtime evidence and authoritative decisions.
// FR: Aucun état de santé ou de décision ne traverse une rotation de clé.
// EN: Health and decision state never leak across credential rotations.
// Called by: acquireEngine; Calls: sync, context.
type engineState struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
	ready                   chan struct{}
	initErr                 error
	refs                    int
	lastAppsecSuccess       int64
	appsecFailures          int64
	lastStreamSuccess       int64
	lastLiveSuccess         int64
	isStartup               uint32
	isCrowdsecStreamHealthy uint32
	updateFailure           int64
	blockedRequests         int64
	lastMetricsPush         time.Time
	startedAt               time.Time
	metricsMutex            sync.Mutex
	tokenMutex              sync.Mutex
	decisions               *decisionStore
}

var engines = struct {
	sync.Mutex
	values map[string]*Bouncer
}{values: make(map[string]*Bouncer)}

// engineFingerprint includes resolved secrets and file bytes, excluding route-local presentation.
// FR: Le condensat reste privé ; une clé ou un certificat remplacé crée un nouvel état.
// EN: The digest stays private; replacing a key or certificate creates fresh state.
// Called by: acquireEngine; Calls: json, sha256, os.ReadFile.
func engineFingerprint(config *configuration.Config) (string, error) {
	resolved := *config
	for _, name := range []string{"CrowdsecLapiKey", "CrowdsecAppsecKey", "CrowdsecCapiMachineID", "CrowdsecCapiPassword"} {
		value, resolveErr := configuration.GetVariable(&resolved, name)
		if resolveErr != nil {
			return "", resolveErr
		}
		reflect.ValueOf(&resolved).Elem().FieldByName(name).SetString(value)
	}
	if resolved.CrowdsecAppsecKey == "" {
		resolved.CrowdsecAppsecKey = resolved.CrowdsecLapiKey
	}
	if resolved.CrowdsecAppsecEnabled && resolved.CrowdsecAppsecScheme == "" {
		resolved.CrowdsecAppsecScheme = resolved.CrowdsecLapiScheme
	}
	if resolved.CrowdsecMode == configuration.AloneMode {
		resolved.CrowdsecLapiScheme = configuration.HTTPS
		resolved.CrowdsecLapiHost = crowdsecCapiHost
		resolved.CrowdsecLapiPath = "/"
		resolved.UpdateIntervalSeconds = 7200
	}
	config = &resolved
	fields := map[string]interface{}{}
	v := reflect.ValueOf(*config)
	typ := v.Type()
	for n := 0; n < v.NumField(); n++ {
		name := typ.Field(n).Name
		if strings.HasPrefix(name, "Crowdsec") || strings.HasPrefix(name, "Redis") || strings.HasPrefix(name, "Update") || name == "StreamDecisionCapacity" || name == "FailureGraceSeconds" || name == "HTTPTimeoutSeconds" || name == "MetricsUpdateIntervalSeconds" || name == "DefaultDecisionSeconds" {
			fields[name] = v.Field(n).Interface()
			if strings.HasSuffix(name, "File") && v.Field(n).Kind() == reflect.String && v.Field(n).String() != "" {
				data, err := os.ReadFile(v.Field(n).String())
				if err != nil {
					return "", err
				}
				fields[name+"Contents"] = data
			}
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// acquireEngine synchronizes initialization once and counts every handler context independently.
// FR: Le premier handler n'est jamais le propriétaire de la durée de vie des autres.
// EN: The first handler never owns the lifetime of the remaining handlers.
// Called by: New; Calls: handleStreamTicker, runEngineWorker, releaseEngine.
func acquireEngine(ctx context.Context, b *Bouncer, cfg *configuration.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := engineFingerprint(cfg)
	if err != nil {
		return err
	}
	engines.Lock()
	owner := engines.values[key]
	fresh := owner == nil
	if fresh {
		b.cacheClient.Namespace = "pareflux:v2:" + key + ":"
		runtime, cancel := context.WithCancel(context.Background())
		b.engine = &engineState{ctx: runtime, cancel: cancel, ready: make(chan struct{}), isStartup: 1, lastMetricsPush: time.Now(), startedAt: time.Now(), decisions: newDecisionStore(int(cfg.StreamDecisionCapacity))}
		owner = b
		engines.values[key] = owner
	}
	owner.engine.refs++
	engines.Unlock()
	b.engine = owner.engine
	b.cacheClient = owner.cacheClient
	var once sync.Once
	release := func() { once.Do(func() { releaseEngine(key, owner) }) }
	if ctx.Done() != nil {
		go func() { <-ctx.Done(); release() }()
	}

	if fresh {
		if b.crowdsecMode == configuration.AloneMode {
			err = getToken(owner)
		}
		if err == nil && (b.crowdsecMode == configuration.StreamMode || b.crowdsecMode == configuration.AloneMode) {
			handleStreamTicker(owner)
		}
		owner.engine.initErr = err
		close(owner.engine.ready)
		if b.crowdsecMode == configuration.StreamMode || b.crowdsecMode == configuration.AloneMode {
			go runEngineWorker(owner.engine.ctx, time.Duration(cfg.UpdateIntervalSeconds)*time.Second, func() { handleStreamTicker(owner) })
		}
		if cfg.MetricsUpdateIntervalSeconds > 0 {
			go runEngineWorker(owner.engine.ctx, time.Duration(cfg.MetricsUpdateIntervalSeconds)*time.Second, func() { handleMetricsTicker(owner) })
		}
	} else {
		select {
		case <-owner.engine.ready:
			err = owner.engine.initErr
		case <-ctx.Done():
			release()
			return ctx.Err()
		}
	}
	if err != nil {
		release()
		return err
	}
	if ctx.Err() != nil {
		release()
		return ctx.Err()
	}
	return nil
}

// releaseEngine cancels outstanding HTTP calls and periodic work after the last reference.
// FR: Le registre ne réutilise jamais un moteur dont l'arrêt a été demandé.
// EN: The registry never reuses an engine whose shutdown was requested.
// Called by: handler context cancellation; Calls: context cancellation, HTTP idle cleanup.
func releaseEngine(key string, b *Bouncer) {
	engines.Lock()
	defer engines.Unlock()
	b.engine.refs--
	if b.engine.refs == 0 {
		delete(engines.values, key)
		b.engine.cancel()
		b.httpClient.CloseIdleConnections()
		b.httpAppsecClient.CloseIdleConnections()
	}
}

// runEngineWorker serializes ticks and exits with its engine, without detached overlapping work.
// FR: Une opération lente ne lance pas de synchronisations concurrentes.
// EN: Slow operations never spawn overlapping synchronization calls.
// Called by: acquireEngine; Calls: time.Ticker.
func runEngineWorker(ctx context.Context, interval time.Duration, work func()) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			work()
		}
	}
}

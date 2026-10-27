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
// Package crowdsec_bouncer_traefik_plugin implements a middleware that communicates with crowdsec.
// It can cache results in memory or using redis, or even ask crowdsec for every requests.
package pareflux_traefik_gate //nolint:revive,stylecheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"text/template"
	"time"

	cache "github.com/vandman/pareflux-traefik-gate/pkg/cache"
	captcha "github.com/vandman/pareflux-traefik-gate/pkg/captcha"
	configuration "github.com/vandman/pareflux-traefik-gate/pkg/configuration"
	ip "github.com/vandman/pareflux-traefik-gate/pkg/ip"
	logger "github.com/vandman/pareflux-traefik-gate/pkg/logger"
)

const (
	crowdsecAppsecIPHeader   = "X-Crowdsec-Appsec-Ip"
	crowdsecAppsecURIHeader  = "X-Crowdsec-Appsec-Uri"
	crowdsecAppsecHostHeader = "X-Crowdsec-Appsec-Host"
	crowdsecAppsecVerbHeader = "X-Crowdsec-Appsec-Verb"
	crowdsecAppsecHeader     = "X-Crowdsec-Appsec-Api-Key"
	crowdsecAppsecUserAgent  = "X-Crowdsec-Appsec-User-Agent"
	crowdsecLapiHeader       = "X-Api-Key"
	crowdsecLapiRoute        = "v1/decisions"
	crowdsecLapiStreamRoute  = "v1/decisions/stream"
	crowdsecLapiMetricsRoute = "v1/usage-metrics"
	crowdsecCapiHost         = "api.crowdsec.net"
	crowdsecCapiHeader       = "Authorization"
	crowdsecCapiLoginRoute   = "v2/watchers/login"
	crowdsecCapiStreamRoute  = "v2/decisions/stream"
	cacheTimeoutKey          = "updated"
	maxCrowdSecResponseBytes = 16 << 20
	maxAppsecResponseBytes   = 64 << 10
	maxIdleConnections       = 100
	maxIdlePerHost           = 20
)

// CreateConfig creates the default plugin configuration.
func CreateConfig() *configuration.Config {
	return configuration.New()
}

// Bouncer a Bouncer struct.
type Bouncer struct {
	appsecBodyExceptions         []compiledBodyException
	policyKey                    string
	appsecBodyLimitAction        string
	appsecTimeout                time.Duration
	appsecReadTimeout            time.Duration
	appsecMaxConcurrentPerClient int
	engine                       *engineState
	failureGraceSeconds          int64
	next                         http.Handler
	name                         string
	template                     *template.Template

	enabled                 bool
	appsecEnabled           bool
	appsecScheme            string
	appsecHost              string
	appsecPath              string
	appsecKey               string
	appsecFailureBlock      bool
	appsecUnreachableBlock  bool
	appsecBodyLimit         int64
	appsecBodyExcludedTypes []string
	crowdsecScheme          string
	crowdsecHost            string
	crowdsecPath            string
	crowdsecKey             string
	crowdsecMode            string
	crowdsecMachineID       string
	crowdsecPassword        string
	crowdsecScenarios       []string
	streamUnreachableBlock  bool
	updateInterval          int64
	updateMaxFailure        int64
	defaultDecisionTimeout  int64
	remediationStatusCode   int
	remediationCustomHeader string
	forwardedCustomHeader   string
	crowdsecStreamRoute     string
	crowdsecHeader          string
	redisUnreachableBlock   bool
	banTemplate             *htmltemplate.Template
	traceCustomHeader       string
	clientPoolStrategy      *ip.PoolStrategy
	serverPoolStrategy      *ip.PoolStrategy
	httpClient              *http.Client
	httpAppsecClient        *http.Client
	cacheClient             *cache.Client
	captchaClient           *captcha.Client
	log                     *slog.Logger
}

// /////////////////////////////////////////////////////////////////////////////
// New
// FR: Construit le middleware PareFlux et retourne immédiatement un passage
//
//	neutre lorsque le composant est désactivé.
//
// EN: Builds the PareFlux middleware and immediately returns a neutral
//
//	pass-through when the component is disabled.
//
// Called by: Traefik local plugin loader
// Calls: configuration validation, CrowdSec clients, cache and tickers
// /////////////////////////////////////////////////////////////////////////////
//
//nolint:gocyclo
func New(ctx context.Context, next http.Handler, config *configuration.Config, name string) (http.Handler, error) {
	if config.DashboardIntegration || config.DashboardLinkURL != "" {
		if config.Enabled {
			return nil, errors.New("dashboard links require a dedicated middleware instance with enabled=false")
		}
		if config.DashboardIntegration {
			key, err := configuration.GetVariable(config, "DashboardDiscoveryKey")
			if err != nil {
				return nil, err
			}
			handler, menuErr := newDashboardIntegration(next, config.DashboardHost, config.DashboardLinkURL, config.DashboardHealthURL, key, config.DashboardRegistrationAllowed)
			if menuErr != nil {
				return nil, menuErr
			}
			fingerprint, fingerprintErr := engineFingerprint(config)
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			handler.(*dashboardMenu).engineKey = fingerprint
			return handler, nil
		}
		return newDashboardMenu(next, config.DashboardHost, config.DashboardLinkURL)
	}
	config.LogLevel = strings.ToUpper(config.LogLevel)
	log := logger.NewWithFormat(config.LogLevel, config.LogFilePath, config.LogFormat)
	if !config.Enabled {
		return &Bouncer{
			next:    next,
			enabled: false,
			log:     log,
		}, nil
	}
	err := configuration.ValidateParams(config, log)
	if err != nil {
		log.Error("New:validateParams " + err.Error())
		return nil, err
	}

	serverChecker, _ := ip.NewChecker(log, config.ForwardedHeadersTrustedIPs)
	clientChecker, _ := ip.NewChecker(log, config.ClientTrustedIPs)

	var tlsAppsecConfig *tls.Config
	if config.CrowdsecAppsecEnabled {
		tlsAppsecConfig, err = configuration.GetTLSConfigCrowdsec(config, log, true)
		if config.CrowdsecAppsecScheme == "" {
			config.CrowdsecAppsecScheme = config.CrowdsecLapiScheme
		}
		if err != nil {
			log.Error("New:getTLSConfigCrowdsec fail to get tlsAppsecConfig " + err.Error())
			return nil, err
		}
		apiAppsecKey, errAppsecKey := configuration.GetVariable(config, "CrowdsecAppsecKey")
		if errAppsecKey != nil && len(tlsAppsecConfig.Certificates) == 0 {
			log.Info("New:crowdsecLapiKey fail to get CrowdsecAppsecKey and no client certificate setup " + errAppsecKey.Error())
		}
		config.CrowdsecAppsecKey = apiAppsecKey
	}

	var tlsConfig *tls.Config
	crowdsecStreamRoute := ""
	crowdsecHeader := ""
	if config.CrowdsecMode == configuration.AloneMode {
		config.CrowdsecCapiMachineID, _ = configuration.GetVariable(config, "CrowdsecCapiMachineID")
		config.CrowdsecCapiPassword, _ = configuration.GetVariable(config, "CrowdsecCapiPassword")
		config.CrowdsecLapiScheme = configuration.HTTPS
		config.CrowdsecLapiHost = crowdsecCapiHost
		config.CrowdsecLapiPath = "/"
		config.UpdateIntervalSeconds = 7200 // 2 hours
		crowdsecStreamRoute = crowdsecCapiStreamRoute
		crowdsecHeader = crowdsecCapiHeader
	} else {
		crowdsecStreamRoute = crowdsecLapiStreamRoute
		crowdsecHeader = crowdsecLapiHeader
		tlsConfig, err = configuration.GetTLSConfigCrowdsec(config, log, false)
		if err != nil {
			log.Error("New:getTLSConfigCrowdsec fail to get tlsConfig " + err.Error())
			return nil, err
		}
		apiKey, errKey := configuration.GetVariable(config, "CrowdsecLapiKey")
		if errKey != nil && len(tlsConfig.Certificates) == 0 {
			log.Error("New:crowdsecLapiKey fail to get CrowdsecLapiKey and no client certificate setup " + errKey.Error())
			return nil, errKey
		}
		config.CrowdsecLapiKey = apiKey
		if config.CrowdsecAppsecKey == "" {
			config.CrowdsecAppsecKey = apiKey
		}
	}

	var banTemplate *htmltemplate.Template
	if config.BanHTMLFilePath != "" {
		var templateErr error
		banTemplate, templateErr = configuration.GetHTMLTemplate(config.BanHTMLFilePath)
		if templateErr != nil {
			return nil, fmt.Errorf("invalid ban template: %w", templateErr)
		}
	}
	appsecTimeoutSeconds := config.CrowdsecAppsecTimeoutSeconds
	if appsecTimeoutSeconds == 0 {
		appsecTimeoutSeconds = 3
	}

	bouncer := &Bouncer{
		next:     next,
		name:     name,
		template: template.New("PareFluxGate").Delims("[[", "]]"),

		enabled:                      config.Enabled,
		failureGraceSeconds:          config.FailureGraceSeconds,
		crowdsecMode:                 config.CrowdsecMode,
		appsecEnabled:                config.CrowdsecAppsecEnabled,
		appsecBodyLimitAction:        config.CrowdsecAppsecBodyLimitAction,
		appsecTimeout:                time.Duration(appsecTimeoutSeconds) * time.Second,
		appsecReadTimeout:            time.Duration(config.CrowdsecAppsecReadTimeoutSeconds) * time.Second,
		appsecMaxConcurrentPerClient: int(config.CrowdsecAppsecMaxConcurrentPerClient),
		appsecScheme:                 config.CrowdsecAppsecScheme,
		appsecHost:                   config.CrowdsecAppsecHost,
		appsecPath:                   config.CrowdsecAppsecPath,
		appsecKey:                    config.CrowdsecAppsecKey,
		appsecFailureBlock:           config.CrowdsecAppsecFailureBlock,
		appsecUnreachableBlock:       config.CrowdsecAppsecUnreachableBlock,
		appsecBodyLimit:              config.CrowdsecAppsecBodyLimit,
		appsecBodyExcludedTypes:      config.CrowdsecAppsecBodyExcludedContentTypes,
		crowdsecScheme:               config.CrowdsecLapiScheme,
		crowdsecHost:                 config.CrowdsecLapiHost,
		crowdsecPath:                 config.CrowdsecLapiPath,
		crowdsecKey:                  config.CrowdsecLapiKey,
		crowdsecMachineID:            config.CrowdsecCapiMachineID,
		crowdsecPassword:             config.CrowdsecCapiPassword,
		crowdsecScenarios:            config.CrowdsecCapiScenarios,
		streamUnreachableBlock:       config.CrowdsecStreamUnreachableBlock,
		updateInterval:               config.UpdateIntervalSeconds,
		updateMaxFailure:             config.UpdateMaxFailure,
		remediationCustomHeader:      config.RemediationHeadersCustomName,
		forwardedCustomHeader:        config.ForwardedHeadersCustomName,
		defaultDecisionTimeout:       config.DefaultDecisionSeconds,
		remediationStatusCode:        config.RemediationStatusCode,
		redisUnreachableBlock:        config.RedisCacheUnreachableBlock,
		banTemplate:                  banTemplate,
		traceCustomHeader:            config.TraceHeadersCustomName,
		crowdsecStreamRoute:          crowdsecStreamRoute,
		crowdsecHeader:               crowdsecHeader,
		log:                          log,
		serverPoolStrategy: &ip.PoolStrategy{
			Checker: serverChecker,
		},
		clientPoolStrategy: &ip.PoolStrategy{
			Checker: clientChecker,
		},
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        maxIdleConnections,
				MaxIdleConnsPerHost: maxIdlePerHost,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     tlsConfig,
			},
			Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
		},
		httpAppsecClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        maxIdleConnections,
				MaxIdleConnsPerHost: maxIdlePerHost,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     tlsAppsecConfig,
			},
			Timeout: time.Duration(appsecTimeoutSeconds) * time.Second,
		},
		cacheClient:   &cache.Client{},
		captchaClient: &captcha.Client{},
	}

	config.RedisCachePassword, _ = configuration.GetVariable(config, "RedisCachePassword")
	bouncer.cacheClient.New(
		log,
		config.RedisCacheEnabled,
		config.RedisCacheHost,
		config.RedisCachePassword,
		config.RedisCacheDatabase,
	)
	config.CaptchaSiteKey, _ = configuration.GetVariable(config, "CaptchaSiteKey")
	config.CaptchaSecretKey, _ = configuration.GetVariable(config, "CaptchaSecretKey")
	policyData, _ := json.Marshal(config)
	policyHash := sha256.Sum256(policyData)
	bouncer.policyKey = hex.EncodeToString(policyHash[:])
	// FR: Les preuves captcha ne traversent pas des politiques de route différentes.
	// EN: Captcha proofs never cross different route policies.
	captchaCache := &cache.Client{Namespace: "pareflux:captcha:v2:" + bouncer.policyKey + ":"}
	captchaCache.New(log, config.RedisCacheEnabled, config.RedisCacheHost, config.RedisCachePassword, config.RedisCacheDatabase)
	err = bouncer.captchaClient.New(
		log,
		captchaCache,
		&http.Client{
			Transport: &http.Transport{MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second},
			Timeout:   time.Duration(config.HTTPTimeoutSeconds) * time.Second,
		},
		config.CaptchaProvider,
		config.CaptchaCustomJsURL,
		config.CaptchaCustomKey,
		config.CaptchaCustomResponse,
		config.CaptchaCustomValidateURL,
		config.CaptchaSiteKey,
		config.CaptchaSecretKey,
		config.RemediationHeadersCustomName,
		config.CaptchaHTMLFilePath,
		config.CaptchaGracePeriodSeconds,
	)
	if err != nil {
		log.Error("CaptchaClient not valid " + err.Error())
		return nil, err
	}

	compiledExceptions, err := compileBodyExceptions(config.CrowdsecAppsecBodyExceptions)
	if err != nil {
		return nil, err
	}
	bouncer.appsecBodyExceptions = compiledExceptions
	if err := acquireEngine(ctx, bouncer, config); err != nil {
		return nil, err
	}

	bouncer.log.Debug("New initialized mode:" + config.CrowdsecMode)

	return bouncer, nil
}

// /////////////////////////////////////////////////////////////////////////////
// ServeHTTP
// FR: Applique les décisions CrowdSec et AppSec, ou appelle une seule fois le
//
//	handler suivant lorsque PareFlux est désactivé.
//
// EN: Applies CrowdSec and AppSec decisions, or calls the next handler exactly
//
//	once when PareFlux is disabled.
//
// Called by: Traefik HTTP request pipeline
// Calls: cache, CrowdSec LAPI, AppSec and the next handler
// /////////////////////////////////////////////////////////////////////////////
//
//nolint:nestif,gocyclo
func (bouncer *Bouncer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !bouncer.enabled {
		bouncer.next.ServeHTTP(rw, req)
		return
	}

	// FR: Une politique identique déjà exécutée sur l'entrypoint ne réinspecte pas le corps.
	// EN: An identical policy already executed at the entrypoint does not inspect the body again.
	if proof, ok := req.Context().Value(gateContextKey{}).(*gateRequestProof); ok && bouncer.policyKey != "" && proof.gate != nil && proof.gate.policyKey == bouncer.policyKey && proof.gate.engine == bouncer.engine {
		bouncer.next.ServeHTTP(rw, req)
		return
	}
	req = req.WithContext(context.WithValue(req.Context(), gateContextKey{}, &gateRequestProof{gate: bouncer}))
	// A caller must not be able to forge the internal remediation marker.
	bouncer.sanitizeRemediationMarker(req)

	// Here we check for the trusted IPs in the forwardedCustomHeader
	remoteIP, err := ip.GetRemoteIP(req, bouncer.serverPoolStrategy, bouncer.forwardedCustomHeader)
	if err != nil {
		bouncer.log.Error(fmt.Sprintf("ServeHTTP:getRemoteIp ip:%s %s", remoteIP, err.Error()))
		bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonTECH)
		return
	}
	isTrusted, err := bouncer.clientPoolStrategy.Checker.Contains(remoteIP)
	if err != nil {
		bouncer.log.Error(fmt.Sprintf("ServeHTTP:checkerContains ip:%s %s", remoteIP, err.Error()))
		bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonTECH)
		return
	}
	// if our IP is in the trusted list we bypass the next checks
	bouncer.log.Debug(fmt.Sprintf("ServeHTTP ip:%s isTrusted:%v", remoteIP, isTrusted))
	if isTrusted {
		bouncer.next.ServeHTTP(rw, req)
		return
	}

	if bouncer.crowdsecMode == configuration.AppsecMode {
		bouncer.handleNextServeHTTP(rw, req, remoteIP)
		return
	}

	if bouncer.engine != nil && (bouncer.crowdsecMode == configuration.StreamMode || bouncer.crowdsecMode == configuration.AloneMode) {
		if value, found := bouncer.engine.decisions.lookup(remoteIP); found {
			bouncer.handleRemediationServeHTTP(rw, req, remoteIP, value)
			return
		}
		if shouldBlockOnStreamFailure(bouncer) {
			bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonTECH)
		} else {
			bouncer.handleNextServeHTTP(rw, req, remoteIP)
		}
		return
	}
	// TODO This should be simplified
	if bouncer.crowdsecMode != configuration.NoneMode {
		value, cacheErr := bouncer.cacheClient.Get(remoteIP)
		if cacheErr != nil {
			cacheErrString := cacheErr.Error()
			bouncer.log.Debug(fmt.Sprintf("ServeHTTP:Get ip:%s isBanned:false %s", remoteIP, cacheErrString))
			if !bouncer.redisUnreachableBlock && cacheErrString == cache.CacheUnreachable {
				bouncer.log.Error(fmt.Sprintf("ServeHTTP:Get ip:%s redisUnreachable=true", remoteIP))
				bouncer.handleNextServeHTTP(rw, req, remoteIP)
				return
			}
			if cacheErrString != cache.CacheMiss {
				bouncer.log.Error(fmt.Sprintf("ServeHTTP:Get ip:%s %s", remoteIP, cacheErrString))
				bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonTECH)
				return
			}
		} else {
			bouncer.log.Debug(fmt.Sprintf("ServeHTTP ip:%s cache:hit isBanned:%v", remoteIP, value))
			if value == cache.NoBannedValue {
				bouncer.handleNextServeHTTP(rw, req, remoteIP)
			} else {
				bouncer.handleRemediationServeHTTP(rw, req, remoteIP, value)
			}
			return
		}
	}

	// Right here if we cannot join the stream we forbid the request to go on.
	if bouncer.crowdsecMode == configuration.StreamMode || bouncer.crowdsecMode == configuration.AloneMode {
		if shouldBlockOnStreamFailure(bouncer) {
			bouncer.log.Debug(fmt.Sprintf(
				"ServeHTTP isCrowdsecStreamHealthy:false ip:%s updateFailure:%d",
				remoteIP,
				atomic.LoadInt64(&bouncer.engine.updateFailure),
			))
			bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonTECH)
		} else {
			bouncer.handleNextServeHTTP(rw, req, remoteIP)
		}
	} else {
		value, err := handleNoStreamCache(bouncer, remoteIP)
		if err != nil {
			bouncer.log.Debug("handleNoStreamCache:crowdsecQuery " + err.Error())
		}
		if value == cache.NoBannedValue {
			bouncer.handleNextServeHTTP(rw, req, remoteIP)
		} else {
			bouncer.log.Debug(fmt.Sprintf("ServeHTTP:handleNoStreamCache ip:%s isBanned:%v %s", remoteIP, value, err.Error()))
			bouncer.handleRemediationServeHTTP(rw, req, remoteIP, value)
		}
	}
}

// /////////////////////////////////////////////////////////////////////////////
// sanitizeRemediationMarker
// FR: Supprime tout marqueur fourni par le client avant que PareFlux ne décide
//
//	lui-même d'en écrire un pour un rejet.
//
// EN: Removes any client-supplied marker before PareFlux decides whether to
//
//	write one for a rejection.
//
// Called by: ServeHTTP and tests
// Calls: net/http Header
// /////////////////////////////////////////////////////////////////////////////
func (bouncer *Bouncer) sanitizeRemediationMarker(req *http.Request) {
	if bouncer.remediationCustomHeader != "" {
		req.Header.Del(bouncer.remediationCustomHeader)
	}
}

// CUSTOM CODE.
// TODO place in another file.

// Decision Body returned from Crowdsec LAPI.
type Decision struct {
	ID        int    `json:"id"`
	Origin    string `json:"origin"`
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Value     string `json:"value"`
	Duration  string `json:"duration"`
	Scenario  string `json:"scenario"`
	Simulated bool   `json:"simulated"`
}

// Stream Body returned from Crowdsec Stream LAPI.
type Stream struct {
	skipped uint64
	Deleted []Decision `json:"deleted"`
	New     []Decision `json:"new"`
}

// Login Body returned from Crowdsec Login CAPI.
type Login struct {
	Code   int    `json:"code"`
	Token  string `json:"token"`
	Expire string `json:"expire"`
}

// To append Headers we need to call rw.WriteHeader after set any header.
func (bouncer *Bouncer) handleBanServeHTTP(rw http.ResponseWriter, req *http.Request, remoteIP, reason string) {
	if bouncer.engine != nil {
		atomic.AddInt64(&bouncer.engine.blockedRequests, 1)
	}

	if bouncer.remediationCustomHeader != "" {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set(bouncer.remediationCustomHeader, "ban")
		rw.Header().Set(bouncer.remediationCustomHeader, "ban")
	}
	if bouncer.banTemplate == nil {
		rw.WriteHeader(bouncer.remediationStatusCode)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(bouncer.remediationStatusCode)

	if req.Method == http.MethodHead {
		return
	}
	templateData := map[string]string{
		"RemediationReason": reason,
		"ClientIP":          remoteIP,
	}

	if bouncer.traceCustomHeader != "" {
		headerVal := req.Header.Get(bouncer.traceCustomHeader)

		if headerVal != "" {
			templateData["TraceID"] = headerVal
		}
	}

	err := bouncer.banTemplate.Execute(rw, templateData)

	if err != nil {
		bouncer.log.Warn("handleBanServeHTTP could not write template to ResponseWriter: " + err.Error())
	}
}

func (bouncer *Bouncer) handleRemediationServeHTTP(rw http.ResponseWriter, req *http.Request, remoteIP, remediation string) {
	bouncer.log.Debug(fmt.Sprintf("handleRemediationServeHTTP ip:%s remediation:%s", remoteIP, remediation))
	if bouncer.captchaClient.Valid && remediation == cache.CaptchaValue && req.Method != http.MethodHead {
		if bouncer.captchaClient.Check(remoteIP) {
			bouncer.handleNextServeHTTP(rw, req, remoteIP)
			return
		}
		if bouncer.engine != nil {
			atomic.AddInt64(&bouncer.engine.blockedRequests, 1)
		} //  If we serve a captcha that should count as a dropped request.
		bouncer.captchaClient.ServeHTTP(rw, req, remoteIP)
		return
	}
	bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonLAPI)
}

func (bouncer *Bouncer) handleNextServeHTTP(rw http.ResponseWriter, req *http.Request, remoteIP string) {
	if bouncer.appsecEnabled {
		controller := http.NewResponseController(rw)
		budget := bouncer.appsecTimeout
		if budget <= 0 {
			budget = 3 * time.Second
		}
		started := time.Now()
		if bouncer.appsecReadTimeout > 0 {
			_ = controller.SetReadDeadline(started.Add(budget))
		}
		err := appsecQuery(bouncer, remoteIP, req)
		if bouncer.appsecReadTimeout > 0 {
			_ = controller.SetReadDeadline(started.Add(bouncer.appsecReadTimeout))
		}
		defer releaseInspectionBody(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				rw.Header().Set("Connection", "close")
				rw.WriteHeader(http.StatusRequestTimeout)
				return
			}
			if errors.Is(err, errBodyTooLarge) {
				rw.WriteHeader(http.StatusRequestEntityTooLarge)
			} else if errors.Is(err, errInspectionCapacity) || errors.Is(err, errInspectionUnavailable) {
				rw.Header().Set("Retry-After", "1")
				rw.WriteHeader(http.StatusServiceUnavailable)
			} else {
				bouncer.handleBanServeHTTP(rw, req, remoteIP, configuration.ReasonAPPSEC)
			}
			return
		}
		if p, ok := req.Context().Value(gateContextKey{}).(*gateRequestProof); ok && p.appsecPartial {
			rw.Header().Set("X-Pareflux-Appsec-Inspection", "partial")
			bouncer.log.Warn("AppSec partial inspection: explicit streaming or upload policy")
		}
	}
	bouncer.next.ServeHTTP(rw, req)
}

func handleStreamTicker(bouncer *Bouncer) {
	if err := handleStreamCache(bouncer); err != nil {
		failures := atomic.AddInt64(&bouncer.engine.updateFailure, 1)
		healthy := crowdsecStreamIsHealthy(bouncer)
		bouncer.log.Warn(fmt.Sprintf("handleStreamTicker updateFailure:%d isCrowdsecStreamHealthy:%t %s", failures, healthy, err.Error()))
		if bouncer.updateMaxFailure != -1 && failures > bouncer.updateMaxFailure && healthy {
			atomic.StoreUint32(&bouncer.engine.isCrowdsecStreamHealthy, 0)
			bouncer.log.Error(fmt.Sprintf("handleStreamTicker:error updateFailure:%d %s", failures, err.Error()))
		}
	} else {
		atomic.StoreUint32(&bouncer.engine.isCrowdsecStreamHealthy, 1)
		atomic.StoreInt64(&bouncer.engine.updateFailure, 0)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// crowdsecStreamIsHealthy
// FR: Lit atomiquement l'état partagé du flux LAPI afin que les requêtes et le
//
//	ticker ne provoquent pas de course de données.
//
// EN: Atomically reads the shared LAPI stream state so requests and the ticker
//
//	do not race.
//
// Called by: ServeHTTP and handleStreamTicker
// Calls: sync/atomic
// /////////////////////////////////////////////////////////////////////////////
func crowdsecStreamIsHealthy(bouncer *Bouncer) bool {
	if bouncer.engine == nil {
		return false
	}
	return atomic.LoadUint32(&bouncer.engine.isCrowdsecStreamHealthy) == 1
}

// /////////////////////////////////////////////////////////////////////////////
// shouldBlockOnStreamFailure
// FR: Conserve les décisions déjà en cache mais choisit explicitement si une
//
//	panne du flux LAPI doit bloquer les adresses absentes du cache.
//
// EN: Keeps cached decisions enforced while explicitly choosing whether a LAPI
//
//	stream outage blocks addresses missing from the cache.
//
// Called by: ServeHTTP and tests
// Calls: crowdsecStreamIsHealthy
// /////////////////////////////////////////////////////////////////////////////
func shouldBlockOnStreamFailure(bouncer *Bouncer) bool {
	if !bouncer.streamUnreachableBlock {
		return false
	}
	if bouncer.engine == nil {
		return true
	}
	if atomic.LoadInt64(&bouncer.engine.lastStreamSuccess) == 0 {
		return true
	}
	age := time.Now().Unix() - atomic.LoadInt64(&bouncer.engine.lastStreamSuccess)
	return !crowdsecStreamIsHealthy(bouncer) && age > bouncer.failureGraceSeconds
}

func handleMetricsTicker(bouncer *Bouncer) {
	if err := reportMetrics(bouncer); err != nil {
		bouncer.log.Error("handleMetricsTicker:reportMetrics " + err.Error())
	}
}

// We are now in none or live mode.
func handleNoStreamCache(bouncer *Bouncer, remoteIP string) (string, error) {
	isLiveMode := bouncer.crowdsecMode == configuration.LiveMode
	routeURL := url.URL{
		Scheme:   bouncer.crowdsecScheme,
		Host:     bouncer.crowdsecHost,
		Path:     bouncer.crowdsecPath + crowdsecLapiRoute,
		RawQuery: url.Values{"ip": []string{remoteIP}}.Encode(),
	}
	body, err := crowdsecQuery(bouncer, routeURL.String(), nil)
	if err != nil {
		return cache.BannedValue, err
	}

	if bytes.Equal(body, []byte("null")) {
		if bouncer.engine != nil {
			atomic.StoreInt64(&bouncer.engine.lastLiveSuccess, time.Now().Unix())
		}
		if isLiveMode {
			bouncer.cacheClient.Set(remoteIP, cache.NoBannedValue, bouncer.defaultDecisionTimeout)
		}
		return cache.NoBannedValue, nil
	}

	var decisions []Decision
	err = json.Unmarshal(body, &decisions)
	if err != nil {
		return cache.BannedValue, fmt.Errorf("handleNoStreamCache:parseBody %w", err)
	}
	if len(decisions) == 0 {
		if bouncer.engine != nil {
			atomic.StoreInt64(&bouncer.engine.lastLiveSuccess, time.Now().Unix())
		}
		if isLiveMode {
			bouncer.cacheClient.Set(remoteIP, cache.NoBannedValue, bouncer.defaultDecisionTimeout)
		}
		return cache.NoBannedValue, nil
	}
	var decision Decision
	for _, d := range decisions {
		decision = d
		if decision.Type == "ban" {
			break
		}
	}
	duration, err := time.ParseDuration(decision.Duration)
	if err != nil {
		return cache.BannedValue, fmt.Errorf("handleNoStreamCache:parseDuration %w", err)
	}
	var value string
	switch decision.Type {
	case "ban":
		value = cache.BannedValue
	case "captcha":
		value = cache.CaptchaValue
	default:
		bouncer.log.Info("handleStreamCache:unknownType " + decision.Type)
	}
	if isLiveMode && bouncer.defaultDecisionTimeout > 0 {
		durationSecond := int64(duration.Seconds())
		if bouncer.defaultDecisionTimeout < durationSecond {
			durationSecond = bouncer.defaultDecisionTimeout
		}
		bouncer.cacheClient.Set(remoteIP, value, durationSecond)
	}
	if bouncer.engine != nil {
		atomic.StoreInt64(&bouncer.engine.lastLiveSuccess, time.Now().Unix())
	}
	return value, errors.New("handleNoStreamCache:banned")
}

// /////////////////////////////////////////////////////////////////////////////
// getToken
// FR: Authentifie la machine auprès de la CAPI sans relancer récursivement
//
//	une authentification refusée.
//
// EN: Authenticates the machine against CAPI without recursively retrying a
//
//	rejected authentication request.
//
// Called by: New, crowdsecQueryWithRefresh
// Calls: crowdsecQueryWithRefresh
// /////////////////////////////////////////////////////////////////////////////
func getToken(bouncer *Bouncer) error {
	loginURL := url.URL{
		Scheme: bouncer.crowdsecScheme,
		Host:   bouncer.crowdsecHost,
		Path:   crowdsecCapiLoginRoute,
	}

	loginData, err := json.Marshal(map[string]interface{}{"machine_id": bouncer.crowdsecMachineID, "password": bouncer.crowdsecPassword, "scenarios": bouncer.crowdsecScenarios})
	if err != nil {
		return err
	}

	body, err := crowdsecQueryWithRefresh(bouncer, loginURL.String(), loginData, false)
	if err != nil {
		return err
	}
	var login Login
	err = json.Unmarshal(body, &login)
	if err != nil {
		return fmt.Errorf("getToken:parsingBody %w", err)
	}
	if login.Code == http.StatusOK && len(login.Token) > 0 {
		if bouncer.engine != nil {
			bouncer.engine.tokenMutex.Lock()
			defer bouncer.engine.tokenMutex.Unlock()
		}
		bouncer.crowdsecKey = login.Token
		return nil
	}
	bouncer.log.Warn(fmt.Sprintf("getToken statusCode:%d", login.Code))
	return fmt.Errorf("getToken statusCode:%d", login.Code)
}

func handleStreamCache(bouncer *Bouncer) error {
	// Full snapshots avoid losing deltas when old and new credential configurations overlap.
	route := url.URL{Scheme: bouncer.crowdsecScheme, Host: bouncer.crowdsecHost, Path: bouncer.crowdsecPath + bouncer.crowdsecStreamRoute, RawQuery: "startup=true"}
	var stream Stream
	_, err := crowdsecQueryConsume(bouncer, route.String(), nil, true, func(reader io.Reader) error {
		var decodeErr error
		stream, decodeErr = decodeDecisionSnapshot(reader, bouncer.engine.decisions.capacity)
		return decodeErr
	})
	if err != nil {
		if errors.Is(err, errDecisionCapacity) {
			bouncer.engine.decisions.mu.Lock()
			bouncer.engine.decisions.overflow = true
			bouncer.engine.decisions.mu.Unlock()
		}
		return err
	}
	beforeSkipped := bouncer.engine.decisions.skippedCount()
	if err = bouncer.engine.decisions.apply(stream, true); err != nil {
		return err
	}
	if skipped := bouncer.engine.decisions.skippedCount() - beforeSkipped; skipped > 0 {
		bouncer.log.Warn("CrowdSec decisions skipped", "count", skipped)
	}
	atomic.StoreInt64(&bouncer.engine.lastStreamSuccess, time.Now().Unix())
	atomic.StoreUint32(&bouncer.engine.isStartup, 0)
	return nil
}

// /////////////////////////////////////////////////////////////////////////////
// crowdsecQuery
// FR: Exécute une requête CrowdSec avec au plus un renouvellement de jeton
//
//	CAPI et conserve la méthode ainsi que le corps lors de la reprise.
//
// EN: Executes a CrowdSec request with at most one CAPI token refresh while
//
//	preserving the method and body on retry.
//
// Called by: decision, stream and metrics synchronizers
// Calls: crowdsecQueryWithRefresh
// /////////////////////////////////////////////////////////////////////////////
func crowdsecQuery(bouncer *Bouncer, stringURL string, data []byte) ([]byte, error) {
	return crowdsecQueryWithRefresh(bouncer, stringURL, data, true)
}

// /////////////////////////////////////////////////////////////////////////////
// crowdsecQueryWithRefresh
// FR: Applique la limite de renouvellement d'authentification afin qu'un 401
//
//	sur la route de connexion ou sur la seconde tentative retourne une erreur.
//
// EN: Enforces the authentication refresh bound so a 401 from the login route
//
//	or the second attempt returns an error.
//
// Called by: crowdsecQuery, getToken
// Calls: getToken, http.Client.Do
// /////////////////////////////////////////////////////////////////////////////
func crowdsecQueryWithRefresh(bouncer *Bouncer, stringURL string, data []byte, allowTokenRefresh bool) ([]byte, error) {
	return crowdsecQueryConsume(bouncer, stringURL, data, allowTokenRefresh, nil)
}

// crowdsecQueryConsume retains authentication retries while permitting bounded incremental consumers.
// FR: Le consommateur n'est invoqué qu'après validation HTTP, sous le délai total du client.
// EN: The consumer runs only after HTTP validation, under the client's total timeout.
// Called by: crowdsecQueryWithRefresh, handleStreamCache; Calls: HTTP client and consumer.
func crowdsecQueryConsume(bouncer *Bouncer, stringURL string, data []byte, allowTokenRefresh bool, consume func(io.Reader) error) ([]byte, error) {
	var req *http.Request
	if len(data) > 0 {
		req, _ = http.NewRequest(http.MethodPost, stringURL, bytes.NewBuffer(data))
	} else {
		req, _ = http.NewRequest(http.MethodGet, stringURL, nil)
	}
	if bouncer.engine != nil {
		req = req.WithContext(bouncer.engine.ctx)
		bouncer.engine.tokenMutex.Lock()
	}
	req.Header.Add(bouncer.crowdsecHeader, bouncer.crowdsecKey)
	if bouncer.engine != nil {
		bouncer.engine.tokenMutex.Unlock()
	}
	req.Header.Add("User-Agent", "PareFlux-Gate/"+pluginVersion)

	res, err := bouncer.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crowdsecQuery:unreachable url:%s %w", stringURL, err)
	}
	defer func() {
		if err = res.Body.Close(); err != nil {
			bouncer.log.Error("crowdsecQuery:closeBody " + err.Error())
		}
	}()
	if res.StatusCode == http.StatusUnauthorized && bouncer.crowdsecMode == configuration.AloneMode && allowTokenRefresh {
		if errToken := getToken(bouncer); errToken != nil {
			return nil, fmt.Errorf("crowdsecQuery:renewToken url:%s %w", stringURL, errToken)
		}
		return crowdsecQueryConsume(bouncer, stringURL, data, false, consume)
	}

	// Check if the status code starts with 2
	statusStr := strconv.Itoa(res.StatusCode)
	if len(statusStr) < 1 || statusStr[0] != '2' {
		return nil, fmt.Errorf("crowdsecQuery method:%s url:%s, statusCode:%d (expected: 2xx)", req.Method, stringURL, res.StatusCode)
	}

	responseLimit := int64(maxCrowdSecResponseBytes)
	if bouncer.engine != nil && strings.Contains(req.URL.Path, bouncer.crowdsecStreamRoute) && bouncer.crowdsecStreamRoute != "" {
		responseLimit = int64(bouncer.engine.decisions.capacity)*512 + (1 << 20)
		if responseLimit < maxCrowdSecResponseBytes {
			responseLimit = maxCrowdSecResponseBytes
		}
		if responseLimit > 256<<20 {
			responseLimit = 256 << 20
		}
	}
	if consume != nil {
		limited := &io.LimitedReader{R: res.Body, N: responseLimit + 1}
		if consumeErr := consume(limited); consumeErr != nil {
			return nil, consumeErr
		}
		if limited.N <= 0 {
			return nil, fmt.Errorf("CrowdSec response size exceeded")
		}
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("crowdsecQuery:readBody %w", err)
	}
	if int64(len(body)) > responseLimit {
		return nil, fmt.Errorf("crowdsecQuery:response exceeds %d bytes", responseLimit)
	}
	return body, nil
}

func reportMetrics(bouncer *Bouncer) error {
	bouncer.engine.metricsMutex.Lock()
	defer bouncer.engine.metricsMutex.Unlock()

	now := time.Now()
	currentCount := atomic.LoadInt64(&bouncer.engine.blockedRequests)
	windowSizeSeconds := int(now.Sub(bouncer.engine.lastMetricsPush).Seconds())

	bouncer.log.Debug(fmt.Sprintf("reportMetrics: blocked_requests=%d window_size=%ds", currentCount, windowSizeSeconds))

	metrics := map[string]interface{}{
		"remediation_components": []map[string]interface{}{
			{
				"version": pluginVersion,
				"type":    "bouncer",
				"name":    "traefik_plugin",
				"metrics": []map[string]interface{}{
					{
						"items": []map[string]interface{}{
							{
								"name":  "dropped",
								"value": currentCount,
								"unit":  "request",
								"labels": map[string]string{
									"type": "traefik_plugin",
								},
							},
						},
						"meta": map[string]interface{}{
							"window_size_seconds": windowSizeSeconds,
							"utc_now_timestamp":   now.Unix(),
						},
					},
				},
				"utc_startup_timestamp": bouncer.engine.startedAt.Unix(),
				"feature_flags":         []string{},
				"os": map[string]string{
					"name":    "unknown",
					"version": "unknown",
				},
			},
		},
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("reportMetrics:marshal %w", err)
	}

	metricsURL := url.URL{
		Scheme: bouncer.crowdsecScheme,
		Host:   bouncer.crowdsecHost,
		Path:   bouncer.crowdsecPath + crowdsecLapiMetricsRoute,
	}

	_, err = crowdsecQuery(bouncer, metricsURL.String(), data)
	if err != nil {
		return fmt.Errorf("reportMetrics:query %w", err)
	}

	atomic.StoreInt64(&bouncer.engine.blockedRequests, 0)
	bouncer.engine.lastMetricsPush = now
	return nil
}

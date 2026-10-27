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
// Package cache implements utility routines for manipulating cache.
// It supports currently local file and redis cache.
package cache

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	simpleredis "github.com/vandman/pareflux-traefik-gate/pkg/simpleredis"
	ttl_map "gitlab.nikolov.fr/application/pareflux-ttl-cache"
)

const (
	// BannedValue Banned string.
	BannedValue = "t"
	// NoBannedValue No banned string.
	NoBannedValue = "f"
	// CaptchaValue Need captcha string.
	CaptchaValue = "c"
	// CaptchaDoneValue Captcha done string.
	CaptchaDoneValue = "d"
	// CacheMiss error string when cache is miss.
	CacheMiss = "cache:miss"
	// CacheUnreachable error string when cache is unreachable.
	CacheUnreachable = "cache:unreachable"
)

// localCache belongs to one engine and never shares entries with another configuration.
// FR: Les changements de clé créent un cache indépendant.
// EN: Credential changes create an independent cache.
// Called by: Client.New; Calls: ttl_map.
type localCache struct {
	values *ttl_map.Cache
	once   sync.Once
}

// store lazily initializes standalone test clients as well as production clients.
// FR: L'initialisation concurrente ne remplace jamais un cache déjà utilisé.
// EN: Concurrent initialization never replaces an existing cache.
// Called by: get, set, delete; Calls: sync.Once.
func (c *localCache) store() *ttl_map.Cache {
	c.once.Do(func() { c.values = ttl_map.New() })
	return c.values
}
func (c *localCache) get(key string) (string, error) {
	v, ok := c.store().Get(key)
	s, valid := v.(string)
	if ok && valid && s != "" {
		return s, nil
	}
	return "", errors.New(CacheMiss)
}
func (c *localCache) set(key, value string, duration int64) { c.store().Set(key, value, duration) }
func (c *localCache) delete(key string)                     { c.store().Del(key) }

type redisCache struct {
	log   *slog.Logger
	redis *simpleredis.SimpleRedis
}

func (rc redisCache) get(key string) (string, error) {
	value, err := rc.redis.Get(key)
	valueString := string(value)
	if err == nil && len(valueString) > 0 {
		return valueString, nil
	}
	if err == nil {
		return "", errors.New(CacheMiss)
	}
	errRedisMessage := err.Error()
	if errRedisMessage == simpleredis.RedisMiss {
		return "", errors.New(CacheMiss)
	}
	if errRedisMessage == simpleredis.RedisUnreachable {
		return "", errors.New(CacheUnreachable)
	}
	return "", err
}

func (rc redisCache) set(key, value string, duration int64) {
	if err := rc.redis.Set(key, []byte(value), duration); err != nil {
		rc.log.Error("cache:setDecisionRedisCache" + err.Error())
	}
}

func (rc redisCache) delete(key string) {
	if err := rc.redis.Del(key); err != nil {
		rc.log.Error("cache:deleteDecisionRedisCache " + err.Error())
	}
}

type cacheInterface interface {
	set(key, value string, duration int64)
	get(key string) (string, error)
	delete(key string)
}

// Client Cache client.
type Client struct {
	Namespace string
	cache     cacheInterface
	log       *slog.Logger
}

// New Initialize cache client.
func (c *Client) New(log *slog.Logger, isRedis bool, host, pass, database string) {
	c.log = log
	if isRedis {
		redis := &simpleredis.SimpleRedis{}
		redis.Init(host, pass, database)
		c.cache = &redisCache{log: log, redis: redis}
	} else {
		c.cache = &localCache{}
	}
	c.log.Debug(fmt.Sprintf("cache:New initialized isRedis:%v", isRedis))
}

// Delete delete decision in cache.
func (c *Client) Delete(key string) {
	c.log.Debug(fmt.Sprintf("cache:Delete key:%v", key))
	c.cache.delete(c.Namespace + key)
}

// Get check in the cache if the IP has the banned / not banned value.
// Otherwise return with an error to add the IP in cache if we are on.
func (c *Client) Get(key string) (string, error) {
	c.log.Debug(fmt.Sprintf("cache:Get key:%v", key))
	return c.cache.get(c.Namespace + key)
}

// Set update the cache with the IP as key and the value banned / not banned.
func (c *Client) Set(key string, value string, duration int64) {
	c.log.Debug(fmt.Sprintf("cache:Set key:%v value:%v duration:%vs", key, value, duration))
	c.cache.set(c.Namespace+key, value, duration)
}

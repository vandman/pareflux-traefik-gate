// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux concurrent TTL cache
// FR: Fournit un cache mémoire concurrent sans goroutine d'arrière-plan afin
//
//	de rester prévisible dans les plugins Traefik interprétés par Yaegi.
//
// EN: Provides a concurrent in-memory cache without background goroutines so
//
//	it remains predictable inside Traefik plugins interpreted by Yaegi.
//
// Called by: PareFlux Gate and other Go consumers
// Calls: container/list, sync.RWMutex, time.Time
// /////////////////////////////////////////////////////////////////////////////
package ttlcache

import (
	"container/list"
	"math"
	"sync"
	"time"
)

const defaultCapacity = 65_536

// /////////////////////////////////////////////////////////////////////////////
// entry
// FR: Conserve une valeur, son échéance et sa génération afin qu'une lecture
//
//	expirée ne supprime jamais un renouvellement concurrent.
//
// EN: Stores a value, its expiry, and its generation so an expired read never
//
//	deletes a concurrent renewal.
//
// Called by: Cache.Set, Cache.Get, Cache.PurgeExpired
// Calls: none
// /////////////////////////////////////////////////////////////////////////////
type entry struct {
	value      any
	expiresAt  int64
	generation uint64
}

// /////////////////////////////////////////////////////////////////////////////
// Cache
// FR: Protège une table clé-valeur bornée et une file d'éviction par un verrou
//
//	lecture-écriture, sans processus d'entretien permanent.
//
// EN: Protects a bounded key-value table and eviction queue with a read-write
//
//	lock, without a permanent maintenance process.
//
// Called by: New and application consumers
// Calls: container/list, entry
// /////////////////////////////////////////////////////////////////////////////
type Cache struct {
	mu         sync.RWMutex
	items      map[string]entry
	positions  map[string]*list.Element
	order      *list.List
	generation uint64
	capacity   int
	now        func() time.Time
}

// /////////////////////////////////////////////////////////////////////////////
// New
// FR: Construit un cache vide utilisant l'horloge système.
// EN: Builds an empty cache using the system clock.
// Called by: application consumers
// Calls: newWithClock
// /////////////////////////////////////////////////////////////////////////////
func New() *Cache {
	return NewWithCapacity(defaultCapacity)
}

// /////////////////////////////////////////////////////////////////////////////
// NewWithCapacity
// FR: Construit un cache borne afin qu'une succession de clés contrôlées par
//
//	un client ne puisse pas épuiser la mémoire du processus.
//
// EN: Builds a bounded cache so a stream of client-controlled keys cannot
//
//	exhaust process memory.
//
// Called by: New and application consumers
// Calls: container/list
// /////////////////////////////////////////////////////////////////////////////
func NewWithCapacity(capacity int) *Cache {
	if capacity < 1 {
		capacity = defaultCapacity
	}
	return &Cache{
		items:     make(map[string]entry),
		positions: make(map[string]*list.Element),
		order:     list.New(),
		capacity:  capacity,
	}
}

// /////////////////////////////////////////////////////////////////////////////
// newWithClock
// FR: Construit un cache avec une horloge injectée pour rendre les tests
//
//	d'expiration déterministes.
//
// EN: Builds a cache with an injected clock to make expiration tests
//
//	deterministic.
//
// Called by: New and tests
// Calls: container/list
// /////////////////////////////////////////////////////////////////////////////
func newWithClock(now func() time.Time) *Cache {
	return &Cache{
		items:     make(map[string]entry),
		positions: make(map[string]*list.Element),
		order:     list.New(),
		capacity:  defaultCapacity,
		now:       now,
	}
}

// /////////////////////////////////////////////////////////////////////////////
// nowUnixNano
// FR: Lit directement l'horloge système en production et n'utilise l'horloge
//
//	injectée que pour les tests déterministes.
//
// EN: Reads the system clock directly in production and uses the injected
//
//	clock only for deterministic tests.
//
// Called by: Cache.Set, Cache.Get, Cache.PurgeExpired
// Calls: time.Now, time.Time.UnixNano
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) nowUnixNano() int64 {
	if c.now == nil {
		return time.Now().UnixNano()
	}
	return c.now().UnixNano()
}

// /////////////////////////////////////////////////////////////////////////////
// Set
// FR: Enregistre une valeur pour une durée exprimée en secondes ; zéro ne
//
//	modifie pas le cache et une durée négative crée une entrée permanente.
//
// EN: Stores a value for a duration expressed in seconds; zero leaves the
//
//	cache unchanged and a negative duration creates a permanent entry.
//
// Called by: application consumers
// Calls: Cache.evictOldestLocked, container/list, time.Time.UnixNano
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) Set(key string, value any, ttlSeconds int64) {
	if ttlSeconds == 0 {
		return
	}

	expiresAt := int64(-1)
	if ttlSeconds > 0 {
		now := c.nowUnixNano()
		if ttlSeconds > math.MaxInt64/int64(time.Second) {
			expiresAt = math.MaxInt64
		} else {
			duration := ttlSeconds * int64(time.Second)
			if now > math.MaxInt64-duration {
				expiresAt = math.MaxInt64
			} else {
				expiresAt = now + duration
			}
		}
	}

	c.mu.Lock()
	current, exists := c.items[key]
	if !exists && len(c.items) >= c.capacity {
		c.evictOldestLocked()
	}
	c.generation++
	if exists {
		current.value = value
		current.expiresAt = expiresAt
		current.generation = c.generation
		c.order.MoveToBack(c.positions[key])
		c.items[key] = current
	} else {
		element := c.order.PushBack(key)
		c.positions[key] = element
		c.items[key] = entry{
			value:      value,
			expiresAt:  expiresAt,
			generation: c.generation,
		}
	}
	c.mu.Unlock()
}

// /////////////////////////////////////////////////////////////////////////////
// Get
// FR: Retourne une valeur valide et supprime paresseusement une entrée expirée
//
//	seulement si sa génération n'a pas changé entre lecture et suppression.
//
// EN: Returns a valid value and lazily removes an expired entry only when its
//
//	generation did not change between reading and deletion.
//
// Called by: application consumers
// Calls: time.Time.UnixNano
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()
	if !found {
		return nil, false
	}

	if item.expiresAt < 0 || item.expiresAt > c.nowUnixNano() {
		return item.value, true
	}

	c.mu.Lock()
	current, stillPresent := c.items[key]
	if stillPresent && current.generation == item.generation {
		delete(c.items, key)
		if element, exists := c.positions[key]; exists {
			delete(c.positions, key)
			c.order.Remove(element)
		}
	}
	c.mu.Unlock()
	return nil, false
}

// /////////////////////////////////////////////////////////////////////////////
// Del
// FR: Supprime une clé si elle existe.
// EN: Deletes a key when it exists.
// Called by: application consumers
// Calls: none
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) Del(key string) {
	c.mu.Lock()
	if _, exists := c.items[key]; exists {
		delete(c.items, key)
		if element, positioned := c.positions[key]; positioned {
			delete(c.positions, key)
			c.order.Remove(element)
		}
	}
	c.mu.Unlock()
}

// /////////////////////////////////////////////////////////////////////////////
// Len
// FR: Retourne le nombre physique d'entrées, y compris celles expirées qui
//
//	n'ont pas encore été consultées ou purgées.
//
// EN: Returns the physical entry count, including expired entries that have
//
//	not yet been read or purged.
//
// Called by: metrics and tests
// Calls: none
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) Len() int {
	c.mu.RLock()
	length := len(c.items)
	c.mu.RUnlock()
	return length
}

// /////////////////////////////////////////////////////////////////////////////
// PurgeExpired
// FR: Supprime au plus `limit` entrées expirées ; une limite non positive
//
//	autorise le nettoyage complet sans lancer de goroutine permanente.
//
// EN: Deletes at most `limit` expired entries; a non-positive limit permits a
//
//	complete cleanup without starting a permanent goroutine.
//
// Called by: application-controlled maintenance and tests
// Calls: time.Time.UnixNano
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) PurgeExpired(limit int) int {
	now := c.nowUnixNano()

	c.mu.Lock()
	removed := c.purgeExpiredLocked(now, limit)
	c.mu.Unlock()

	return removed
}

// /////////////////////////////////////////////////////////////////////////////
// purgeExpiredLocked
// FR: Nettoie les entrées expirées lorsque l'appelant détient déjà le verrou
//
//	d'écriture.
//
// EN: Removes expired entries while the caller already owns the write lock.
//
// Called by: Cache.PurgeExpired
// Calls: none
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) purgeExpiredLocked(now int64, limit int) int {
	removed := 0
	for key, item := range c.items {
		if item.expiresAt >= 0 && item.expiresAt <= now {
			delete(c.items, key)
			if element, exists := c.positions[key]; exists {
				delete(c.positions, key)
				c.order.Remove(element)
			}
			removed++
			if limit > 0 && removed >= limit {
				break
			}
		}
	}
	return removed
}

// /////////////////////////////////////////////////////////////////////////////
// evictOldestLocked
// FR: Retire l'entrée la plus ancienne via la tête de file lorsque l'appelant
//
//	détient le verrou d'écriture, sans parcourir le cache.
//
// EN: Removes the oldest entry from the queue head while the caller owns the
//
//	write lock, without scanning the cache.
//
// Called by: Cache.Set
// Calls: list.List.Front, list.List.Remove
// /////////////////////////////////////////////////////////////////////////////
func (c *Cache) evictOldestLocked() {
	oldest := c.order.Front()
	if oldest == nil {
		return
	}
	key, valid := oldest.Value.(string)
	if valid {
		delete(c.items, key)
		delete(c.positions, key)
	}
	c.order.Remove(oldest)
}

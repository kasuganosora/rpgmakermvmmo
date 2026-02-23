package local

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("cache: key not found")

// Config holds LocalCache settings.
type Config struct {
	GCInterval time.Duration
}

// entry holds a cached string value with an optional expiry.
type entry struct {
	data     string
	expireAt time.Time
	noExpiry bool
}

func (e *entry) expired() bool {
	return !e.noExpiry && time.Now().After(e.expireAt)
}

// LocalCache is an in-process cache implementing the Cache interface.
type LocalCache struct {
	mu         sync.Mutex // guards kvStore SetNX atomically
	kv         sync.Map   // key → *entry
	hashes     sync.Map   // key → *sync.Map (field → string)
	sets       sync.Map   // key → *lockedSet
	zsets      sync.Map   // key → *zset
	lists      sync.Map   // key → *lockedList
	gcInterval time.Duration
	stopGC     chan struct{}
}

// NewCache creates a LocalCache and starts the background GC goroutine.
func NewCache(cfg Config) (*LocalCache, error) {
	interval := cfg.GCInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	c := &LocalCache{
		gcInterval: interval,
		stopGC:     make(chan struct{}),
	}
	go c.runGC()
	return c, nil
}

// Close stops the background GC goroutine.
func (c *LocalCache) Close() {
	close(c.stopGC)
}

func (c *LocalCache) runGC() {
	ticker := time.NewTicker(c.gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.kv.Range(func(k, v interface{}) bool {
				if e, ok := v.(*entry); ok && e.expired() {
					c.kv.Delete(k)
				}
				return true
			})
		case <-c.stopGC:
			return
		}
	}
}

// ---- KV ----

func (c *LocalCache) Get(_ context.Context, key string) (string, error) {
	v, ok := c.kv.Load(key)
	if !ok {
		return "", ErrNotFound
	}
	e := v.(*entry)
	if e.expired() {
		c.kv.Delete(key)
		return "", ErrNotFound
	}
	return e.data, nil
}

func (c *LocalCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	e := &entry{data: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	} else {
		e.noExpiry = true
	}
	c.kv.Store(key, e)
	return nil
}

func (c *LocalCache) Del(_ context.Context, keys ...string) error {
	for _, k := range keys {
		c.kv.Delete(k)
	}
	return nil
}

func (c *LocalCache) Exists(_ context.Context, key string) (bool, error) {
	v, ok := c.kv.Load(key)
	if !ok {
		return false, nil
	}
	e := v.(*entry)
	if e.expired() {
		c.kv.Delete(key)
		return false, nil
	}
	return true, nil
}

func (c *LocalCache) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.kv.Load(key); ok {
		if e, ok2 := v.(*entry); ok2 && !e.expired() {
			return false, nil
		}
	}
	e := &entry{data: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	} else {
		e.noExpiry = true
	}
	c.kv.Store(key, e)
	return true, nil
}

func (c *LocalCache) Expire(_ context.Context, key string, ttl time.Duration) error {
	v, ok := c.kv.Load(key)
	if !ok {
		return ErrNotFound
	}
	e := v.(*entry)
	if e.expired() {
		c.kv.Delete(key)
		return ErrNotFound
	}
	newEntry := &entry{data: e.data, expireAt: time.Now().Add(ttl)}
	c.kv.Store(key, newEntry)
	return nil
}

// ---- Hash ----

func (c *LocalCache) getOrCreateHash(key string) *sync.Map {
	v, _ := c.hashes.LoadOrStore(key, &sync.Map{})
	return v.(*sync.Map)
}

func (c *LocalCache) HSet(_ context.Context, key, field, value string) error {
	c.getOrCreateHash(key).Store(field, value)
	return nil
}

func (c *LocalCache) HGet(_ context.Context, key, field string) (string, error) {
	h := c.getOrCreateHash(key)
	v, ok := h.Load(field)
	if !ok {
		return "", ErrNotFound
	}
	return v.(string), nil
}

func (c *LocalCache) HGetAll(_ context.Context, key string) (map[string]string, error) {
	h := c.getOrCreateHash(key)
	result := make(map[string]string)
	h.Range(func(k, v interface{}) bool {
		result[k.(string)] = v.(string)
		return true
	})
	return result, nil
}

func (c *LocalCache) HDel(_ context.Context, key string, fields ...string) error {
	h := c.getOrCreateHash(key)
	for _, f := range fields {
		h.Delete(f)
	}
	return nil
}

// ---- Set ----

type lockedSet struct {
	mu      sync.RWMutex
	members map[string]struct{}
}

func (c *LocalCache) getOrCreateSet(key string) *lockedSet {
	v, _ := c.sets.LoadOrStore(key, &lockedSet{members: make(map[string]struct{})})
	return v.(*lockedSet)
}

func (c *LocalCache) SAdd(_ context.Context, key string, members ...string) error {
	s := c.getOrCreateSet(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range members {
		s.members[m] = struct{}{}
	}
	return nil
}

func (c *LocalCache) SRem(_ context.Context, key string, members ...string) error {
	s := c.getOrCreateSet(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range members {
		delete(s.members, m)
	}
	return nil
}

func (c *LocalCache) SMembers(_ context.Context, key string) ([]string, error) {
	s := c.getOrCreateSet(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, 0, len(s.members))
	for m := range s.members {
		result = append(result, m)
	}
	return result, nil
}

func (c *LocalCache) SIsMember(_ context.Context, key, member string) (bool, error) {
	s := c.getOrCreateSet(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.members[member]
	return ok, nil
}

// ---- ZSet ----

type zEntry struct {
	member string
	score  float64
}

type zset struct {
	mu      sync.Mutex
	entries []zEntry // sorted by score descending
}

func (c *LocalCache) getOrCreateZSet(key string) *zset {
	v, _ := c.zsets.LoadOrStore(key, &zset{})
	return v.(*zset)
}

func (c *LocalCache) ZAdd(_ context.Context, key string, score float64, member string) error {
	z := c.getOrCreateZSet(key)
	z.mu.Lock()
	defer z.mu.Unlock()
	// Update existing or insert
	for i, e := range z.entries {
		if e.member == member {
			z.entries[i].score = score
			sort.Slice(z.entries, func(a, b int) bool { return z.entries[a].score > z.entries[b].score })
			return nil
		}
	}
	z.entries = append(z.entries, zEntry{member: member, score: score})
	sort.Slice(z.entries, func(a, b int) bool { return z.entries[a].score > z.entries[b].score })
	return nil
}

func (c *LocalCache) ZRevRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	z := c.getOrCreateZSet(key)
	z.mu.Lock()
	defer z.mu.Unlock()
	n := int64(len(z.entries))
	if start >= n {
		return nil, nil
	}
	if stop < 0 || stop >= n {
		stop = n - 1
	}
	result := make([]string, 0, stop-start+1)
	for i := start; i <= stop; i++ {
		result = append(result, z.entries[i].member)
	}
	return result, nil
}

func (c *LocalCache) ZScore(_ context.Context, key, member string) (float64, error) {
	z := c.getOrCreateZSet(key)
	z.mu.Lock()
	defer z.mu.Unlock()
	for _, e := range z.entries {
		if e.member == member {
			return e.score, nil
		}
	}
	return 0, ErrNotFound
}

// ---- List ----

type lockedList struct {
	mu   sync.Mutex
	data []string
}

func (c *LocalCache) getOrCreateList(key string) *lockedList {
	v, _ := c.lists.LoadOrStore(key, &lockedList{})
	return v.(*lockedList)
}

func (c *LocalCache) LPush(_ context.Context, key string, values ...string) error {
	l := c.getOrCreateList(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	// LPush: prepend in order (last value ends up at index 0)
	for _, v := range values {
		l.data = append([]string{v}, l.data...)
	}
	return nil
}

func (c *LocalCache) LRange(_ context.Context, key string, start, stop int64) ([]string, error) {
	l := c.getOrCreateList(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	n := int64(len(l.data))
	if start >= n {
		return nil, nil
	}
	if stop < 0 || stop >= n {
		stop = n - 1
	}
	result := make([]string, stop-start+1)
	copy(result, l.data[start:stop+1])
	return result, nil
}

func (c *LocalCache) LTrim(_ context.Context, key string, start, stop int64) error {
	l := c.getOrCreateList(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	n := int64(len(l.data))
	if start >= n {
		l.data = nil
		return nil
	}
	if stop < 0 || stop >= n {
		stop = n - 1
	}
	l.data = l.data[start : stop+1]
	return nil
}

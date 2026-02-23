package cache

import (
	"context"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/cache/local"
	cacheredis "github.com/kasuganosora/rpgmakermvmmo/server/cache/redis"
)

// Cache defines the KV / Hash / Set / ZSet / List operations.
type Cache interface {
	// KV
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Hash
	HSet(ctx context.Context, key, field, value string) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error

	// Set
	SAdd(ctx context.Context, key string, members ...string) error
	SRem(ctx context.Context, key string, members ...string) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SIsMember(ctx context.Context, key, member string) (bool, error)

	// ZSet
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZScore(ctx context.Context, key, member string) (float64, error)

	// List
	LPush(ctx context.Context, key string, values ...string) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LTrim(ctx context.Context, key string, start, stop int64) error
}

// Message is a received pub/sub message.
type Message struct {
	Channel string
	Payload string
}

// PubSub defines channel publish/subscribe operations.
type PubSub interface {
	Publish(ctx context.Context, channel, message string) error
	Subscribe(ctx context.Context, channels ...string) (<-chan *Message, func(), error)
}

// CacheConfig holds configuration for both Redis and LocalCache.
type CacheConfig struct {
	RedisAddr       string        `mapstructure:"redis_addr"`
	RedisPassword   string        `mapstructure:"redis_password"`
	RedisDB         int           `mapstructure:"redis_db"`
	LocalGCInterval time.Duration `mapstructure:"local_gc_interval"`
	LocalPubSubBuf  int           `mapstructure:"local_pubsub_buf"`
}

// NewCache returns a Cache backed by Redis if RedisAddr is set,
// otherwise returns an in-process LocalCache.
func NewCache(cfg CacheConfig) (Cache, error) {
	if cfg.RedisAddr != "" {
		return cacheredis.NewCache(cacheredis.Config{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
	}
	return local.NewCache(local.Config{
		GCInterval: cfg.LocalGCInterval,
	})
}

// NewPubSub returns a PubSub backed by Redis if RedisAddr is set,
// otherwise returns an in-process LocalPubSub wrapped in an adapter.
func NewPubSub(cfg CacheConfig) (PubSub, error) {
	bufSize := cfg.LocalPubSubBuf
	if bufSize <= 0 {
		bufSize = 256
	}
	if cfg.RedisAddr != "" {
		rps, err := cacheredis.NewPubSub(cacheredis.Config{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		if err != nil {
			return nil, err
		}
		return &redisPubSubAdapter{ps: rps}, nil
	}
	return &localPubSubAdapter{ps: local.NewPubSub(bufSize)}, nil
}

// ---- adapters to bridge sub-package message types to cache.Message ----

type localPubSubAdapter struct {
	ps *local.LocalPubSub
}

func (a *localPubSubAdapter) Publish(ctx context.Context, channel, message string) error {
	return a.ps.Publish(ctx, channel, message)
}

func (a *localPubSubAdapter) Subscribe(ctx context.Context, channels ...string) (<-chan *Message, func(), error) {
	localCh, cancel, err := a.ps.Subscribe(ctx, channels...)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan *Message, 256)
	go func() {
		defer close(out)
		for msg := range localCh {
			out <- &Message{Channel: msg.Channel, Payload: msg.Payload}
		}
	}()
	return out, cancel, nil
}

type redisPubSubAdapter struct {
	ps *cacheredis.RedisPubSub
}

func (a *redisPubSubAdapter) Publish(ctx context.Context, channel, message string) error {
	return a.ps.Publish(ctx, channel, message)
}

func (a *redisPubSubAdapter) Subscribe(ctx context.Context, channels ...string) (<-chan *Message, func(), error) {
	redisCh, cancel, err := a.ps.Subscribe(ctx, channels...)
	if err != nil {
		return nil, nil, err
	}
	out := make(chan *Message, 256)
	go func() {
		defer close(out)
		for msg := range redisCh {
			out <- &Message{Channel: msg.Channel, Payload: msg.Payload}
		}
	}()
	return out, cancel, nil
}

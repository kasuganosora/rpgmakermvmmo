package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("cache: key not found")

// Config holds Redis connection settings.
type Config struct {
	Addr     string
	Password string
	DB       int
}

// RedisCache implements the Cache interface backed by Redis.
type RedisCache struct {
	client *goredis.Client
}

// NewCache creates a Redis-backed cache.
func NewCache(cfg Config) (*RedisCache, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisCache{client: client}, nil
}

// ---- KV ----

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrNotFound
	}
	return v, err
}

func (r *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (r *RedisCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// ---- Hash ----

func (r *RedisCache) HSet(ctx context.Context, key, field, value string) error {
	return r.client.HSet(ctx, key, field, value).Err()
}

func (r *RedisCache) HGet(ctx context.Context, key, field string) (string, error) {
	v, err := r.client.HGet(ctx, key, field).Result()
	if errors.Is(err, goredis.Nil) {
		return "", ErrNotFound
	}
	return v, err
}

func (r *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

func (r *RedisCache) HDel(ctx context.Context, key string, fields ...string) error {
	return r.client.HDel(ctx, key, fields...).Err()
}

// ---- Set ----

func (r *RedisCache) SAdd(ctx context.Context, key string, members ...string) error {
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return r.client.SAdd(ctx, key, args...).Err()
}

func (r *RedisCache) SRem(ctx context.Context, key string, members ...string) error {
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return r.client.SRem(ctx, key, args...).Err()
}

func (r *RedisCache) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, key).Result()
}

func (r *RedisCache) SIsMember(ctx context.Context, key, member string) (bool, error) {
	return r.client.SIsMember(ctx, key, member).Result()
}

// ---- ZSet ----

func (r *RedisCache) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return r.client.ZAdd(ctx, key, goredis.Z{Score: score, Member: member}).Err()
}

func (r *RedisCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRevRange(ctx, key, start, stop).Result()
}

func (r *RedisCache) ZScore(ctx context.Context, key, member string) (float64, error) {
	v, err := r.client.ZScore(ctx, key, member).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, ErrNotFound
	}
	return v, err
}

// ---- List ----

func (r *RedisCache) LPush(ctx context.Context, key string, values ...string) error {
	args := make([]interface{}, len(values))
	for i, v := range values {
		args[i] = v
	}
	return r.client.LPush(ctx, key, args...).Err()
}

func (r *RedisCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

func (r *RedisCache) LTrim(ctx context.Context, key string, start, stop int64) error {
	return r.client.LTrim(ctx, key, start, stop).Err()
}

// ---- PubSub ----

// RedisMessage is the message type returned by RedisPubSub.Subscribe.
type RedisMessage struct {
	Channel string
	Payload string
}

// RedisPubSub wraps the Redis PubSub client.
type RedisPubSub struct {
	client *goredis.Client
}

// NewPubSub creates a Redis-backed PubSub.
func NewPubSub(cfg Config) (*RedisPubSub, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisPubSub{client: client}, nil
}

func (r *RedisPubSub) Publish(ctx context.Context, channel, message string) error {
	return r.client.Publish(ctx, channel, message).Err()
}

func (r *RedisPubSub) Subscribe(ctx context.Context, channels ...string) (<-chan *RedisMessage, func(), error) {
	ps := r.client.Subscribe(ctx, channels...)
	ch := make(chan *RedisMessage, 256)

	go func() {
		defer close(ch)
		for msg := range ps.Channel() {
			ch <- &RedisMessage{Channel: msg.Channel, Payload: msg.Payload}
		}
	}()

	cancel := func() {
		_ = ps.Close()
	}
	return ch, cancel, nil
}

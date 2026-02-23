package local

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *LocalCache {
	c, err := NewCache(Config{GCInterval: time.Minute})
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

func TestGetSet(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	err := c.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	v, err := c.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", v)
}

func TestGetMissing(t *testing.T) {
	c := newTestCache(t)
	_, err := c.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestTTLExpiry(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	err := c.Set(ctx, "ttl_key", "val", 10*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	_, err = c.Get(ctx, "ttl_key")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDel(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	_ = c.Set(ctx, "k", "v", 0)
	_ = c.Del(ctx, "k")
	_, err := c.Get(ctx, "k")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestExists(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	_ = c.Set(ctx, "k", "v", 0)
	exists, err := c.Exists(ctx, "k")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSetNX(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	ok, err := c.SetNX(ctx, "lock", "owner", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = c.SetNX(ctx, "lock", "other", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok) // already held
}

func TestHash(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.HSet(ctx, "h", "f1", "v1"))
	require.NoError(t, c.HSet(ctx, "h", "f2", "v2"))

	v, err := c.HGet(ctx, "h", "f1")
	require.NoError(t, err)
	assert.Equal(t, "v1", v)

	all, err := c.HGetAll(ctx, "h")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"f1": "v1", "f2": "v2"}, all)

	require.NoError(t, c.HDel(ctx, "h", "f1"))
	_, err = c.HGet(ctx, "h", "f1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSet(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.SAdd(ctx, "s", "a", "b", "c"))
	members, err := c.SMembers(ctx, "s")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, members)

	ok, err := c.SIsMember(ctx, "s", "b")
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, c.SRem(ctx, "s", "b"))
	ok, _ = c.SIsMember(ctx, "s", "b")
	assert.False(t, ok)
}

func TestZSet(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.ZAdd(ctx, "z", 100, "alice"))
	require.NoError(t, c.ZAdd(ctx, "z", 200, "bob"))
	require.NoError(t, c.ZAdd(ctx, "z", 50, "carol"))

	members, err := c.ZRevRange(ctx, "z", 0, -1)
	require.NoError(t, err)
	assert.Equal(t, []string{"bob", "alice", "carol"}, members)

	score, err := c.ZScore(ctx, "z", "alice")
	require.NoError(t, err)
	assert.Equal(t, float64(100), score)
}

func TestList(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.LPush(ctx, "l", "c", "b", "a"))
	items, err := c.LRange(ctx, "l", 0, -1)
	require.NoError(t, err)
	// LPush "c" then "b" then "a" → head = a, b, c
	assert.Equal(t, []string{"a", "b", "c"}, items)

	require.NoError(t, c.LTrim(ctx, "l", 0, 1))
	items, _ = c.LRange(ctx, "l", 0, -1)
	assert.Equal(t, []string{"a", "b"}, items)
}

package local

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPubSubBasic(t *testing.T) {
	ps := NewPubSub(16)
	ctx := context.Background()

	ch, cancel, err := ps.Subscribe(ctx, "test-channel")
	require.NoError(t, err)
	defer cancel()

	err = ps.Publish(ctx, "test-channel", "hello")
	require.NoError(t, err)

	select {
	case msg := <-ch:
		assert.Equal(t, "test-channel", msg.Channel)
		assert.Equal(t, "hello", msg.Payload)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestPubSubUnsubscribe(t *testing.T) {
	ps := NewPubSub(16)
	ctx := context.Background()

	ch, cancel, err := ps.Subscribe(ctx, "ch")
	require.NoError(t, err)

	cancel() // unsubscribe

	// Channel should be closed
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed after cancel")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after cancel")
	}

	// Publish to unsubscribed channel should not block
	err = ps.Publish(ctx, "ch", "msg")
	assert.NoError(t, err)
}

func TestPubSubMultipleSubscribers(t *testing.T) {
	ps := NewPubSub(16)
	ctx := context.Background()

	ch1, cancel1, _ := ps.Subscribe(ctx, "broadcast")
	ch2, cancel2, _ := ps.Subscribe(ctx, "broadcast")
	defer cancel1()
	defer cancel2()

	require.NoError(t, ps.Publish(ctx, "broadcast", "world"))

	for _, ch := range []<-chan *LocalMessage{ch1, ch2} {
		select {
		case msg := <-ch:
			assert.Equal(t, "world", msg.Payload)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("subscriber did not receive message")
		}
	}
}

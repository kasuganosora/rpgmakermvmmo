package local

import (
	"context"
	"sync"
)

// LocalMessage is an in-process pub/sub message.
type LocalMessage struct {
	Channel string
	Payload string
}

type subscriber struct {
	ch    chan *LocalMessage
	unsub func()
}

// LocalPubSub is an in-process fan-out pub/sub implementation.
type LocalPubSub struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscriber
	bufSize     int
}

// NewPubSub creates a new LocalPubSub with the given per-subscriber buffer size.
func NewPubSub(bufSize int) *LocalPubSub {
	if bufSize <= 0 {
		bufSize = 256
	}
	return &LocalPubSub{
		subscribers: make(map[string][]*subscriber),
		bufSize:     bufSize,
	}
}

// Publish sends a message to all subscribers of the given channel.
func (ps *LocalPubSub) Publish(_ context.Context, channel, message string) error {
	msg := &LocalMessage{Channel: channel, Payload: message}
	ps.mu.RLock()
	subs := ps.subscribers[channel]
	ps.mu.RUnlock()
	for _, s := range subs {
		select {
		case s.ch <- msg:
		default:
			// Drop message if buffer is full (non-blocking)
		}
	}
	return nil
}

// Subscribe returns a channel of messages for the given channels, and a cancel function.
func (ps *LocalPubSub) Subscribe(_ context.Context, channels ...string) (<-chan *LocalMessage, func(), error) {
	ch := make(chan *LocalMessage, ps.bufSize)
	subs := make([]*subscriber, len(channels))

	ps.mu.Lock()
	for i, c := range channels {
		s := &subscriber{ch: ch}
		ps.subscribers[c] = append(ps.subscribers[c], s)
		subs[i] = s
	}
	ps.mu.Unlock()

	cancel := func() {
		ps.mu.Lock()
		defer ps.mu.Unlock()
		for i, c := range channels {
			list := ps.subscribers[c]
			for j, sub := range list {
				if sub == subs[i] {
					ps.subscribers[c] = append(list[:j], list[j+1:]...)
					break
				}
			}
		}
		close(ch)
	}

	return ch, cancel, nil
}

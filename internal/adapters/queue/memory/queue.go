package memory

import (
	"context"
	"sync"

	"github.com/media-vault-sync/internal/core/services"
)

const MaxAttempts = 3

type subscription struct {
	topic      string
	providerID string
	handler    services.MessageHandler
}

type pendingMessage struct {
	msg      services.Message
	attempts int
}

type InMemoryQueue struct {
	mu            sync.RWMutex
	clock         services.Clock
	subscriptions map[string]*subscription
	pending       []pendingMessage
}

func NewInMemoryQueue(clock services.Clock) *InMemoryQueue {
	return &InMemoryQueue{
		clock:         clock,
		subscriptions: make(map[string]*subscription),
		pending:       make([]pendingMessage, 0),
	}
}

func (q *InMemoryQueue) Publish(ctx context.Context, msg services.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if msg.DeliverAt.IsZero() {
		msg.DeliverAt = q.clock.Now()
	}

	q.pending = append(q.pending, pendingMessage{msg: msg, attempts: 0})
	return nil
}

func (q *InMemoryQueue) Subscribe(ctx context.Context, subscriptionID string, topic string, providerID string, handler services.MessageHandler) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.subscriptions[subscriptionID] = &subscription{
		topic:      topic,
		providerID: providerID,
		handler:    handler,
	}
	return nil
}

func (q *InMemoryQueue) Unsubscribe(subscriptionID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.subscriptions, subscriptionID)
	return nil
}

func (q *InMemoryQueue) Tick(ctx context.Context) (delivered int, requeued int) {
	q.mu.Lock()
	now := q.clock.Now()

	var ready []pendingMessage
	var stillPending []pendingMessage

	for _, pm := range q.pending {
		if !pm.msg.DeliverAt.After(now) {
			ready = append(ready, pm)
		} else {
			stillPending = append(stillPending, pm)
		}
	}
	q.pending = stillPending
	q.mu.Unlock()

	var toRequeue []pendingMessage

	for _, pm := range ready {
		q.mu.RLock()
		var matchedHandler services.MessageHandler
		for _, sub := range q.subscriptions {
			if sub.topic != pm.msg.Topic {
				continue
			}
			msgProviderID := pm.msg.Metadata["providerID"]
			if sub.providerID != "" && sub.providerID != msgProviderID {
				continue
			}
			matchedHandler = sub.handler
			break
		}
		q.mu.RUnlock()

		if matchedHandler == nil {
			pm.attempts++
			if pm.attempts < MaxAttempts {
				toRequeue = append(toRequeue, pm)
			}
			continue
		}

		err := matchedHandler(ctx, pm.msg)
		if err != nil {
			pm.attempts++
			if pm.attempts < MaxAttempts {
				toRequeue = append(toRequeue, pm)
				requeued++
			}
		} else {
			delivered++
		}
	}

	if len(toRequeue) > 0 {
		q.mu.Lock()
		q.pending = append(q.pending, toRequeue...)
		q.mu.Unlock()
	}

	return delivered, requeued
}

func (q *InMemoryQueue) Process(ctx context.Context) (totalDelivered int) {
	for {
		delivered, requeued := q.Tick(ctx)
		totalDelivered += delivered
		if delivered == 0 && requeued == 0 {
			break
		}
	}
	return totalDelivered
}

func (q *InMemoryQueue) PendingCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.pending)
}

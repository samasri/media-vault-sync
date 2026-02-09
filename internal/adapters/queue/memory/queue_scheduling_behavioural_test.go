package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/media-vault-sync/internal/adapters/queue/memory"
	"github.com/media-vault-sync/internal/core/services"
)

func TestQueue_ScheduledMessageNotDeliveredBeforeTime(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := services.NewFakeClock(baseTime)
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var received []services.Message

	err := q.Subscribe(ctx, "sub1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		received = append(received, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	deliverAt := baseTime.Add(5 * time.Second)
	err = q.Publish(ctx, services.Message{
		MessageID: "delayed-msg",
		Topic:     "usersync",
		Payload:   []byte(`{"test":"data"}`),
		Metadata:  map[string]string{"providerID": "p1"},
		DeliverAt: deliverAt,
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	q.Process(ctx)
	if len(received) != 0 {
		t.Errorf("message should not be delivered before DeliverAt, got %d messages", len(received))
	}

	clock.Advance(3 * time.Second)
	q.Process(ctx)
	if len(received) != 0 {
		t.Errorf("message should not be delivered 3s before DeliverAt (need 5s), got %d messages", len(received))
	}

	clock.Advance(2 * time.Second)
	q.Process(ctx)
	if len(received) != 1 {
		t.Errorf("message should be delivered at DeliverAt, got %d messages", len(received))
	}
}

func TestQueue_ScheduledMessageDeliveredAfterTimeAdvances(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := services.NewFakeClock(baseTime)
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var received []services.Message

	err := q.Subscribe(ctx, "sub1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		received = append(received, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	deliverAt := baseTime.Add(10 * time.Second)
	err = q.Publish(ctx, services.Message{
		MessageID: "delayed-msg",
		Topic:     "usersync",
		Payload:   []byte(`{"test":"data"}`),
		Metadata:  map[string]string{"providerID": "p1"},
		DeliverAt: deliverAt,
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	clock.Advance(15 * time.Second)
	delivered := q.Process(ctx)

	if delivered != 1 {
		t.Errorf("expected 1 message delivered after time advanced past DeliverAt, got %d", delivered)
	}
	if len(received) != 1 {
		t.Errorf("subscriber should have received 1 message, got %d", len(received))
	}
	if received[0].MessageID != "delayed-msg" {
		t.Errorf("expected message ID 'delayed-msg', got %s", received[0].MessageID)
	}
}

func TestQueue_ImmediateMessageDeliveredRightAway(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := services.NewFakeClock(baseTime)
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var received []services.Message

	err := q.Subscribe(ctx, "sub1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		received = append(received, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	err = q.Publish(ctx, services.Message{
		MessageID: "immediate-msg",
		Topic:     "usersync",
		Payload:   []byte(`{"test":"data"}`),
		Metadata:  map[string]string{"providerID": "p1"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	delivered := q.Process(ctx)

	if delivered != 1 {
		t.Errorf("expected 1 immediate message delivered, got %d", delivered)
	}
	if len(received) != 1 {
		t.Errorf("subscriber should have received 1 message, got %d", len(received))
	}
}

func TestQueue_MultipleScheduledMessagesDeliverInOrder(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := services.NewFakeClock(baseTime)
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var received []string

	err := q.Subscribe(ctx, "sub1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		received = append(received, msg.MessageID)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	q.Publish(ctx, services.Message{
		MessageID: "msg-at-10s",
		Topic:     "usersync",
		Metadata:  map[string]string{"providerID": "p1"},
		DeliverAt: baseTime.Add(10 * time.Second),
	})
	q.Publish(ctx, services.Message{
		MessageID: "msg-at-5s",
		Topic:     "usersync",
		Metadata:  map[string]string{"providerID": "p1"},
		DeliverAt: baseTime.Add(5 * time.Second),
	})
	q.Publish(ctx, services.Message{
		MessageID: "msg-at-15s",
		Topic:     "usersync",
		Metadata:  map[string]string{"providerID": "p1"},
		DeliverAt: baseTime.Add(15 * time.Second),
	})

	clock.Advance(5 * time.Second)
	q.Process(ctx)
	if len(received) != 1 || received[0] != "msg-at-5s" {
		t.Errorf("at 5s expected [msg-at-5s], got %v", received)
	}

	clock.Advance(5 * time.Second)
	q.Process(ctx)
	if len(received) != 2 || received[1] != "msg-at-10s" {
		t.Errorf("at 10s expected msg-at-10s second, got %v", received)
	}

	clock.Advance(5 * time.Second)
	q.Process(ctx)
	if len(received) != 3 || received[2] != "msg-at-15s" {
		t.Errorf("at 15s expected msg-at-15s third, got %v", received)
	}
}

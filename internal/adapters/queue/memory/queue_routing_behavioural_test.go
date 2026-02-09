package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/media-vault-sync/internal/adapters/queue/memory"
	"github.com/media-vault-sync/internal/core/services"
)

func TestQueue_RoutingByProviderID(t *testing.T) {
	clock := services.NewFakeClock(time.Now())
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var receivedP1 []services.Message

	err := q.Subscribe(ctx, "onprem:p1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		receivedP1 = append(receivedP1, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	err = q.Publish(ctx, services.Message{
		MessageID: "msg-1",
		Topic:     "usersync",
		Payload:   []byte(`{"databaseID":"db1","userID":"user1"}`),
		Metadata:  map[string]string{"providerID": "p1"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	delivered := q.Process(ctx)

	if delivered != 1 {
		t.Errorf("expected 1 message delivered, got %d", delivered)
	}
	if len(receivedP1) != 1 {
		t.Fatalf("expected p1 subscriber to receive 1 message, got %d", len(receivedP1))
	}
	if receivedP1[0].MessageID != "msg-1" {
		t.Errorf("expected message ID 'msg-1', got %s", receivedP1[0].MessageID)
	}
}

func TestQueue_RoutingDoesNotDeliverToWrongProvider(t *testing.T) {
	clock := services.NewFakeClock(time.Now())
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var receivedP1 []services.Message

	err := q.Subscribe(ctx, "onprem:p1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		receivedP1 = append(receivedP1, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	err = q.Publish(ctx, services.Message{
		MessageID: "msg-2",
		Topic:     "usersync",
		Payload:   []byte(`{"databaseID":"db1","userID":"user2"}`),
		Metadata:  map[string]string{"providerID": "p2"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	q.Process(ctx)

	if len(receivedP1) != 0 {
		t.Errorf("expected p1 subscriber to receive 0 messages (message was for p2), got %d", len(receivedP1))
	}
}

func TestQueue_MultipleSubscribersRouteCorrectly(t *testing.T) {
	clock := services.NewFakeClock(time.Now())
	q := memory.NewInMemoryQueue(clock)
	ctx := context.Background()

	var receivedP1, receivedP2 []services.Message

	err := q.Subscribe(ctx, "onprem:p1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		receivedP1 = append(receivedP1, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe p1 failed: %v", err)
	}

	err = q.Subscribe(ctx, "onprem:p2", "usersync", "p2", func(ctx context.Context, msg services.Message) error {
		receivedP2 = append(receivedP2, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe p2 failed: %v", err)
	}

	err = q.Publish(ctx, services.Message{
		MessageID: "msg-for-p1",
		Topic:     "usersync",
		Payload:   []byte(`{"databaseID":"db1","userID":"user1"}`),
		Metadata:  map[string]string{"providerID": "p1"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	err = q.Publish(ctx, services.Message{
		MessageID: "msg-for-p2",
		Topic:     "usersync",
		Payload:   []byte(`{"databaseID":"db1","userID":"user2"}`),
		Metadata:  map[string]string{"providerID": "p2"},
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	delivered := q.Process(ctx)

	if delivered != 2 {
		t.Errorf("expected 2 messages delivered, got %d", delivered)
	}

	if len(receivedP1) != 1 {
		t.Errorf("expected p1 subscriber to receive 1 message, got %d", len(receivedP1))
	} else if receivedP1[0].MessageID != "msg-for-p1" {
		t.Errorf("p1 received wrong message: %s", receivedP1[0].MessageID)
	}

	if len(receivedP2) != 1 {
		t.Errorf("expected p2 subscriber to receive 1 message, got %d", len(receivedP2))
	} else if receivedP2[0].MessageID != "msg-for-p2" {
		t.Errorf("p2 received wrong message: %s", receivedP2[0].MessageID)
	}
}

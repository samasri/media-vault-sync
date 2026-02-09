package services

import (
	"context"
	"time"
)

type Message struct {
	MessageID string
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	DeliverAt time.Time
}

type MessageHandler func(ctx context.Context, msg Message) error

type Queue interface {
	Publish(ctx context.Context, msg Message) error
	Subscribe(ctx context.Context, subscriptionID string, topic string, providerID string, handler MessageHandler) error
	Unsubscribe(subscriptionID string) error
}

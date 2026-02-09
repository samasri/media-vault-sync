package services

import (
	"context"
	"encoding/json"
	"time"
)

const (
	MaxRepairAttempts = 3
	BaseBackoff       = time.Second
)

type EventualConsistencyCheckPayload struct {
	ProviderID string `json:"providerID"`
	DatabaseID string `json:"databaseID"`
	AlbumUID   string `json:"albumUID"`
	Attempt    int    `json:"attempt"`
}

type EventualConsistencyWorker struct {
	albumRepo AlbumRepository
	queue     Queue
	clock     Clock
}

func NewEventualConsistencyWorker(albumRepo AlbumRepository, queue Queue, clock Clock) *EventualConsistencyWorker {
	return &EventualConsistencyWorker{
		albumRepo: albumRepo,
		queue:     queue,
		clock:     clock,
	}
}

func (w *EventualConsistencyWorker) Scan(ctx context.Context) error {
	albums, err := w.albumRepo.FindNeedingRepair(ctx)
	if err != nil {
		return err
	}

	for _, album := range albums {
		payload, err := json.Marshal(EventualConsistencyCheckPayload{
			ProviderID: album.ProviderID,
			DatabaseID: album.DatabaseID,
			AlbumUID:   album.AlbumUID,
			Attempt:    1,
		})
		if err != nil {
			return err
		}

		err = w.queue.Publish(ctx, Message{
			Topic:    "syncconsistencycheck",
			Payload:  payload,
			Metadata: map[string]string{},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

type EventualConsistencyCheckConsumer struct {
	albumRepo AlbumRepository
	queue     Queue
	clock     Clock
}

func NewEventualConsistencyCheckConsumer(albumRepo AlbumRepository, queue Queue, clock Clock) *EventualConsistencyCheckConsumer {
	return &EventualConsistencyCheckConsumer{
		albumRepo: albumRepo,
		queue:     queue,
		clock:     clock,
	}
}

func (c *EventualConsistencyCheckConsumer) Handle(ctx context.Context, msg Message) error {
	var payload EventualConsistencyCheckPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}

	album, err := c.albumRepo.FindByAlbumUID(ctx, payload.ProviderID, payload.DatabaseID, payload.AlbumUID)
	if err != nil {
		return err
	}

	if album == nil || album.Synced {
		return nil
	}

	if payload.Attempt >= MaxRepairAttempts {
		// TODO: send it to a DLQ or a list of work that requires developer intervention
		return nil
	}

	albumManifestUploadPayload, err := json.Marshal(AlbumManifestUploadPayload{
		DatabaseID: payload.DatabaseID,
		AlbumUID:   payload.AlbumUID,
	})
	if err != nil {
		return err
	}

	err = c.queue.Publish(ctx, Message{
		Topic:   "albummanifestupload",
		Payload: albumManifestUploadPayload,
		Metadata: map[string]string{
			"providerID": payload.ProviderID,
		},
	})
	if err != nil {
		return err
	}

	nextPayload, err := json.Marshal(EventualConsistencyCheckPayload{
		ProviderID: payload.ProviderID,
		DatabaseID: payload.DatabaseID,
		AlbumUID:   payload.AlbumUID,
		Attempt:    payload.Attempt + 1,
	})
	if err != nil {
		return err
	}

	backoff := BaseBackoff * time.Duration(1<<(payload.Attempt-1))
	deliverAt := c.clock.Now().Add(backoff)

	return c.queue.Publish(ctx, Message{
		Topic:     "syncconsistencycheck",
		Payload:   nextPayload,
		Metadata:  map[string]string{},
		DeliverAt: deliverAt,
	})
}

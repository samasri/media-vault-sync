# Media Vault Sync

## Overview

This project is a small, interview-friendly replica of a multi-tenant on-prem “Media Vault” syncing albums and videos to a cloud SaaS. It demonstrates a common engineering pattern: event-driven orchestration plus idempotent, retry-safe handlers to make transfers reliable at scale.

The on-prem gateway uses a legacy push-style protocol (C-MOVE/C-STORE semantics) to receive files and forward them to the cloud. The cloud persists manifests and media pointers, rejects unexpected uploads, and uses an eventual-consistency worker to repair incomplete syncs. The code is structured with Clean Architecture boundaries (domain → services → adapters) to keep business logic testable.

## Use Cases

This system has 2 kinds of users:

- Businesses who produce videos at a facility and assign them to users.
- Users who want to see the videos created by the business via our system.

Businesses have a media vault they store all the videos after creating them. The system in this project has 2 objectives:

- Quickly detect when new videos for a user have been added to the media vault
- Sync the new videos to the cloud

## How to run

```sh
go test ./...
go run ./cmd/demo-album-updated/main.go
```

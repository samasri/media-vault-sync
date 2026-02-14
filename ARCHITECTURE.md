# Media Sync Engine Architecture

A media sync and transfer pipeline.

## Terminology Mapping

This codebase uses the following terminology:

| Concept         | Code Term  | Description                        |
|-----------------|------------|------------------------------------|
| Content owner   | user       | The owner of media content         |
| Collection      | album      | A collection of videos             |
| Media file      | video      | Individual media file              |
| On-prem storage | MediaVault | On-prem multi-tenant storage system|
| Tenant          | providerID | Provider/tenant identifier         |
| Database        | databaseID | Database/vault identifier          |

Note: CFind, CMove, CStore are legacy MediaVault protocol verbs retained for compatibility.

## Component Diagram

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLOUD                                          │
│  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐  │
│  │   Cloud API Server  │    │  Sync Consistency   │    │   MySQL/Memory  │  │
│  │  ────────────────── │    │       Worker        │    │   Repositories  │  │
│  │  POST /v1/          │    │  ────────────────── │    │                 │  │
│  │   useralbums        │    │  Scans for unsynced │    │  - albums       │  │
│  │  POST /v1/          │◄───┤  albums & schedules │◄───┤  - videos       │  │
│  │   albummanifestup   │    │  repair checks      │    │  - album_videos │  │
│  │  POST /v1/album/    │    │                     │    │  - objects      │  │
│  │   {uid}/videoupload │    └─────────────────────┘    └─────────────────┘  │
│  └──────────▲──────────┘                                                    │
│             │                                                               │
└─────────────┼───────────────────────────────────────────────────────────────┘
              │ HTTP
              │
┌─────────────┼───────────────────────────────────────────────────────────────┐
│             │                     MESSAGE QUEUE                             │
│  ┌──────────┴────────────────────────────────────────────────────────────┐  │
│  │  Topics: usersync, albummanifestupload, videoupload, syncconsistency  │  │
│  │  ────────────────────────────────────────────────────────────────────  │  │
│  │  • Provider routing via metadata (providerID) - multi-tenancy layer   │  │
│  │  • Each on-prem app subscribes with its configured providerID         │  │
│  │  • Scheduled/delayed delivery                                         │  │
│  │  • At-least-once semantics                                            │  │
│  │  • Max 3 attempts, no DLQ                                             │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
              │
              │ Subscribe
              ▼
┌────────────────────────────────────────────────────────────────────────────────┐
│                             ON-PREM                                            │
│  ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐     │
│  │   On-Prem App       │    │   Video Receiver    │    │   Staging FS    │     │
│  │  ────────────────── │    │  ────────────────── │    │                 │     │
│  │  Consumes:          │    │  HTTP endpoint for  │───►│  Bytes on disk  │     │
│  │  • usersync         │    │  MediaVault C-MOVE  │    │  before upload  │     │
│  │  • albummanifestup  │    │  simulation         │    │                 │     │
│  │  • videoupload      │    │                     │    │                 │     │
│  │                     │    │  VideoSender has    │    │                 │     │
│  │  Consumers have     │    │  providerID injected│    │                 │     │
│  │  providerID injected│    └─────────────────────┘    └─────────────────┘     │
│  └──────────┬──────────┘                                                       │
│             │                                                                  │
│             ▼                                                                  │
│  ┌─────────────────────┐                                                       │
│  │ MediaVaultRegistry  │  Returns DatabaseScopedMediaVault for each databaseID │
│  │  ────────────────── │                                                       │
│  │  • Get(databaseID)  │  Lazy-creates one per database                        │
│  └──────────┬──────────┘                                                       │
│             │                                                                  │
│             ▼                                                                  │
│  ┌─────────────────────┐                                                       │
│  │ DatabaseScopedMV    │  Reads mediavault_config.json JIT                     │
│  │  ────────────────── │                                                       │
│  │  • ListAlbumUIDs()  │  databaseID bound at construction                     │
│  │  • ListVideoUIDs    │  No providerID param (not multi-tenant)               │
│  │  • CMove() sim      │                                                       │
│  └─────────────────────┘                                                       │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

## Topics and Payloads

| Topic                | Payload (JSON)                                | Routing Metadata |
|----------------------|-----------------------------------------------|------------------|
| usersync             | `{databaseID, userID}`                        | `providerID`     |
| albummanifestupload  | `{databaseID, albumUID}`                      | `providerID`     |
| videoupload          | `{databaseID, albumUID}`                      | `providerID`     |
| syncconsistencycheck | `{providerID, databaseID, albumUID, attempt}` | (cloud-consumed) |

Messages include:

- `MessageID`: Unique identifier for idempotency
- `Topic`: Target topic name
- `Payload`: JSON bytes
- `Metadata`: Map including `providerID` for routing
- `DeliverAt`: Scheduled delivery time (for delayed/backoff messages)

## Endpoints

### Cloud API Server

| Method | Path                             | Content-Type             | Response    |
|--------|----------------------------------|--------------------------|-------------|
| POST   | /v1/useralbums                   | application/json         | 200/4xx/5xx |
| POST   | /v1/albummanifestupload          | application/json         | 200/409     |
| POST   | /v1/album/{albumUID}/videoupload | application/octet-stream | 200/409     |

**Request Bodies:**

- `/v1/useralbums`: `{providerID, databaseID, userID, albumUIDs[]}`
- `/v1/albummanifestupload`: `{providerID, databaseID, userID, albumUID, videoUIDs[]}`
- `/v1/album/{albumUID}/videoupload`: Headers: X-Provider-ID, X-Database-ID, X-User-ID, X-Video-UID; Body: binary

### On-Prem Video Receiver

| Method | Path           | Content-Type             | Response    |
|--------|----------------|--------------------------|-------------|
| POST   | /receive-video | application/octet-stream | 200/4xx/5xx |

**Request Details:**

- Headers: X-Provider-ID, X-Database-ID, X-Album-UID, X-Video-UID
- Body: binary data

## Album State Transitions

```text
                    ┌──────────────────┐
                    │   Album Created  │
                    │   synced = true  │
                    └────────┬─────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ Manifest Same │  │ Manifest Changed│  │ Unexpected Vid. │
│ (no-op)       │  │ synced = true   │  │ synced = false  │
└───────────────┘  │ emit videoupload│  └────────┬────────┘
                   └─────────────────┘           │
                                                 │
                                    ┌────────────▼───────────┐
                                    │  Sync Consistency      │
                                    │  Worker detects        │
                                    │  ────────────────────  │
                                    │  Emits albummanifestup │
                                    │  Schedules retry       │
                                    └────────────────────────┘
```

## Idempotency Strategy

### Message Queue

- Messages have unique `MessageID`
- Handlers must be idempotent (same message processed multiple times = same result)
- At-least-once delivery means duplicates are possible

### Database Constraints (Milestone 6)

- `albums`: UNIQUE(provider_id, database_id, album_uid)
- `videos`: UNIQUE(provider_id, database_id, video_uid)
- `album_videos`: UNIQUE(provider_id, database_id, album_uid, video_uid)
- `objects`: UNIQUE(provider_id, database_id, video_uid)

### Endpoint Idempotency

- `/v1/albummanifestupload`: Upsert semantics - creates or updates, safe to retry
- `/v1/album/{uid}/videoupload`: Upsert object record, safe to retry
- `/v1/useralbums`: Only emits `albummanifestupload` for albums that don't exist

## MediaVault Config JIT Behavior

The MediaVault architecture uses a registry pattern:

- `MediaVaultRegistry.Get(databaseID)` returns a `MediaVault` instance scoped to that database
- `DatabaseScopedMediaVault` has the databaseID bound at construction (not passed to methods)
- Each DatabaseScopedMediaVault reads its JSON config file on every call (not cached)
- **MediaVault is not multi-tenant**: The MediaVault interface does not accept `providerID`
  parameters. Multi-tenancy is handled at the queue layer through message routing.

This enables:

1. Testing sync consistency scenarios by modifying config between calls
2. Simulating real-world MediaVault where data changes over time
3. Testing repair loops when config changes after initial manifest build
4. Consumers don't need to pass databaseID or providerID to MediaVault method calls

### Config Schema

```json
{
  "providers": [
    {
      "providerID": "p1",
      "databases": [
        {
          "databaseID": "db1",
          "users": [
            {
              "userID": "user1",
              "albums": [
                { "albumUID": "a1", "videos": ["v1", "v2", "v3"] }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

## Usersync Flow (Milestone 2)

```text
┌─────────────┐       usersync msg       ┌───────────────────┐
│   Queue     │ ──────────────────────►  │  SyncUserConsumer │
│ (usersync)  │                          │    (on-prem)       │
└─────────────┘                          └─────────┬─────────┘
                                                   │
                                    1. mediaVaultRegistry.Get(databaseID)
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │MediaVaultRegistry│
                                         │  (lazy-creates) │
                                         └─────────┬───────┘
                                                   │
                                         returns MediaVault instance
                                                   │
                                                   ▼
                                    2. mediaVault.ListAlbumUIDs(userID)  // no providerID
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │DatabaseScoped   │
                                         │  MediaVault     │
                                         │  (JIT config)   │
                                         └─────────┬───────┘
                                                   │
                                         returns albumUIDs
                                                   │
                                                   ▼
                                    3. POST /v1/useralbums
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │  Cloud API      │
                                         │  (useralbums)   │
                                         └─────────┬───────┘
                                                   │
                               for each albumUID not in repo:
                                 publish albummanifestupload message
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │     Queue       │
                                         │ (albummanifest.)│
                                         └─────────────────┘
```

## AlbumManifestUpload Flow (Milestone 3)

```text
┌─────────────┐   albummanifestupload    ┌────────────────────────────┐
│   Queue     │ ──────────────────────►  │ AlbumManifestUploadConsumer│
│(albummanif.)│                          │    (on-prem)               │
└─────────────┘                          └─────────┬──────────────────┘
                                                   │
                                  1. mediaVaultRegistry.Get(databaseID)
                                                   │
                                                   ▼
                                         ┌──────────────────┐
                                         │MediaVaultRegistry│
                                         └─────────┬────────┘
                                                   │
                                  2. mediaVault.ListVideoUIDs(albumUID)  // no providerID
                                  3. mediaVault.GetUserIDForAlbum(albumUID)  // no providerID
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │DatabaseScoped   │
                                         │  MediaVault     │
                                         │  (JIT config)   │
                                         └─────────┬───────┘
                                                   │
                                         returns videoUIDs, userID
                                                   │
                                                   ▼
                                    4. POST /v1/albummanifestupload
                                       {providerID, databaseID, userID,
                                        albumUID, videoUIDs[]}
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │  Cloud API      │
                                         │  (albummanifest)│
                                         └─────────┬───────┘
                                                   │
                    ┌──────────────────────────────┼──────────────────────────────┐
                    │                              │                              │
                    ▼                              ▼                              ▼
          ┌─────────────────┐          ┌─────────────────┐          ┌─────────────────┐
          │  New Album      │          │ Existing Album  │          │ UserID          │
          │  Insert + emit  │          │ Manifest same   │          │ Mismatch        │
          │  videoupload    │          │ No emit         │          │ Return 409      │
          └─────────────────┘          └─────────────────┘          └─────────────────┘
                                                │
                                                ▼
                                       ┌─────────────────┐
                                       │ Manifest Changed│
                                       │ Update + emit   │
                                       │ videoupload     │
                                       └─────────────────┘
```

## VideoUpload Flow (Milestone 4)

```text
┌─────────────┐      videoupload msg      ┌─────────────────────┐
│   Queue     │ ─────────────────────────►│ VideoUploadConsumer │
│(videoupload)│                           │    (on-prem)        │
└─────────────┘                           └──────────┬──────────┘
                                                     │
                                          1. mediaVaultRegistry.Get(databaseID)
                                                     │
                                                     ▼
                                          ┌──────────────────┐
                                          │MediaVaultRegistry│
                                          └────────┬─────────┘
                                                   │
                                          2. mediaVault.CMove(albumUID)  // no providerID
                                                     │
                                                     ▼
                                          ┌─────────────────┐
                                          │DatabaseScoped   │
                                          │  MediaVault     │
                                          │  (JIT config)   │
                                          └────────┬────────┘
                                                   │
                              for each videoUID in config:
                                                   │
                                                   ▼
                                    2. POST /receive-video
                                       Headers: X-Provider-ID, X-Database-ID,
                                        X-Album-UID, X-Video-UID
                                       Body: binary data (application/octet-stream)
                                                   │
                                                   ▼
                                          ┌─────────────────┐
                                          │  Video Receiver │
                                          │   (on-prem)     │
                                          └────────┬────────┘
                                                   │
                              3. Store bytes in staging folder
                              4. POST /v1/album/{uid}/videoupload
                                                   │
                                                   ▼
                                          ┌─────────────────┐
                                          │  Cloud API      │
                                          │  (videoupload)  │
                                          └────────┬────────┘
                                                   │
                    ┌──────────────────────────────┼──────────────────────┐
                    │                              │                      │
                    ▼                              ▼                      │
          ┌─────────────────┐          ┌─────────────────┐                │
          │ Video in        │          │ Video NOT in    │                │
          │ manifest        │          │ manifest        │                │
          │                 │          │                 │                │
          │ Upsert video    │          │ Set album       │                │
          │ Create object   │          │ synced=false    │                │
          │ Return 200      │          │ Return 409      │                │
          └─────────────────┘          └─────────────────┘                │
                                                                          │
                                          5. Delete from staging ◄────────┘
```

## Sync Consistency Flow (Milestone 5)

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          SYNC CONSISTENCY WORKER                            │
│                                                                             │
│  1. Periodic Scan                                                           │
│     ──────────────                                                          │
│     • FindNeedingRepair() returns albums with synced=false                  │
│     • For each unsynced album, publish syncconsistencycheck message         │
│                                                                             │
│  2. On syncconsistencycheck                                                 │
│     ────────────────────                                                    │
│     • Re-check if album still needs repair (synced=false)                   │
│     • If yes and attempt < 3:                                               │
│       - Publish albummanifestupload message to refresh manifest             │
│       - Schedule another syncconsistencycheck with exponential backoff      │
│                                                                             │
│  Backoff Schedule:                                                          │
│     attempt=1 → retry after 1s                                              │
│     attempt=2 → retry after 2s                                              │
│     attempt=3 → give up (max 3 attempts)                                    │
└─────────────────────────────────────────────────────────────────────────────┘

Repair Flow:
┌──────────────┐        syncconsistencycheck       ┌────────────────────────┐
│    Worker    │ ─────────────────────────────────►│ SC Check Consumer      │
│  (periodic)  │    {providerID, databaseID,       │   (cloud-side)         │
└──────────────┘     albumUID, attempt=1}          └───────────┬────────────┘
                                                               │
                                    still needs repair? ───────┼───────────┐
                                                               │           │
                                                   ┌───────────▼───────┐   │
                                                   │  YES: emit        │   │
                                                   │  albummanifestup  │   │
                                                   │  msg              │   │
                                                   │  Schedule next    │   │
                                                   │  check (backoff)  │   │
                                                   └───────────────────┘   │
                                                                           │
                                                               ┌───────────▼───┐
                                                               │  NO: album    │
                                                               │  is synced,   │
                                                               │  do nothing   │
                                                               └───────────────┘

On albummanifestupload (triggered by repair):
  → MediaVault reads current config (JIT)
  → Manifest updated with new videoUIDs
  → Cloud emits videoupload
  → On-prem CMove sends all current videos
  → If all videos now in manifest: album becomes synced=true
```

## Test Strategy

### Behavioural Tests (Milestones 1-5)

**Queue Tests** (`internal/adapters/queue/memory/`):

| Test File                              | Behavior Verified                      |
|----------------------------------------|----------------------------------------|
| queue_routing_behavioural_test.go      | Provider routing to correct subscriber |
| queue_scheduling_behavioural_test.go   | Scheduled delivery until DeliverAt     |

**Integration Tests** (`tests/`):

| Test File                                                    | Behavior Verified                   |
|--------------------------------------------------------------|-------------------------------------|
| user_sync_new_albums_only_behavioural_test.go                | Only new albums emit manifest msg   |
| albumupload_idempotency_behavioural_test.go                  | Manifest upsert: new/changed only   |
| albumupload_user_immutability_behavioural_test.go            | UserID immutable for existing album |
| end_to_end_happy_path_video_ingest_behavioural_test.go       | Full sync->upload->ingest flow      |
| unexpected_video_marks_unsynced_behavioural_test.go          | Unexpected video marks unsynced     |
| repair_loop_recovers_after_config_change_behavioural_test.go | SC worker repairs album             |
| wiring_end_to_end_behavioural_test.go                        | Wiring composes dependencies        |

### Future Milestones

| Milestone | Test File                                   | Behavior                         |
|-----------|---------------------------------------------|----------------------------------|
| 6         | tests/mysql_schema_and_upsert_integ_test.go | MySQL constraints work correctly |

## MySQL Adapter (Milestone 6)

### Schema

Tables are defined in `migrations/001_initial_schema.sql`:

- **albums**: Core album records with unique constraint on (provider_id, database_id, album_uid)
- **album_videos**: Manifest tracking with unique constraint on (provider_id, database_id, album_uid, video_uid)
- **videos**: Video metadata with unique constraint on (provider_id, database_id, video_uid)
- **objects**: Stored object records with unique constraint on (provider_id, database_id, video_uid)

### Running MySQL

```bash
docker-compose up -d mysql
```

### Running Integration Tests

```bash
export MYSQL_DSN="app:apppassword@tcp(localhost:3306)/media_sync?parseTime=true"
go test ./tests/... -v -run TestMySQL
```

Without MYSQL_DSN set, integration tests are skipped automatically.

### Idempotency Implementation

- `videos` and `objects` tables use `INSERT ... ON DUPLICATE KEY UPDATE` for upsert semantics
- `album_videos` uses delete-then-insert within a transaction for manifest replacement
- Unique constraints prevent duplicate records from being created

## Application Wiring

The application uses a wiring layer to compose dependencies without bloating main functions.

### Cloud Wiring (`internal/app/cloud/`)

```go
cfg := cloudapp.LoadConfig()  // Reads from env vars
app := cloudapp.Wire(cfg, nil) // Returns *App with all dependencies

// App contains:
// - Handler: http.Handler with all routes registered
// - Queue: TickableQueue for message processing
// - AlbumRepo, AlbumVideoRepo, VideoRepo, ObjectRepo
// - EventualConsistencyWorker: periodic scanner
// - EventualConsistencyCheckConsumer: handles syncconsistencycheck messages
```

**Environment Variables:**

- `CLOUD_PORT`: HTTP port (default: 8080)
- `REPO_BACKEND`: "memory" or "mysql" (default: memory)
- `MYSQL_DSN`: MySQL connection string
- `SCAN_INTERVAL`: Sync consistency scan interval (default: 30s)
- `QUEUE_TICK_INTERVAL`: Queue processing interval (default: 100ms)

### On-Prem Wiring (`internal/app/onprem/`)

```go
cfg := onpremapp.LoadConfig()  // Reads from env vars
app := onpremapp.Wire(cfg, nil) // Returns *App with all dependencies

// App contains:
// - Handler: http.Handler with /receive-video
// - Queue: TickableQueue for message processing
// - MediaVaultRegistry: Returns DatabaseScopedMediaVault per databaseID
// - CloudClient: HTTP client to cloud API
// - SyncUserConsumer, AlbumManifestUploadConsumer, VideoUploadConsumer
```

**Environment Variables:**

- `ONPREM_PORT`: HTTP port (default: 8081)
- `MEDIAVAULT_CONFIG_PATH`: Path to MediaVault JSON config (default: mediavault_config.json)
- `STAGING_DIR`: Staging directory for video bytes (default: /tmp/staging)
- `CLOUD_BASE_URL`: Cloud API base URL (default: <http://localhost:8080>)
- `PROVIDER_ID`: Required provider ID for message routing
- `QUEUE_TICK_INTERVAL`: Queue processing interval (default: 100ms)
- `RECEIVER_URL`: Video receiver URL (default: <http://localhost:{PORT}>)

### Testing with WireOptions

Both wiring functions accept optional `WireOptions` for dependency injection in tests:

```go
cloudOpts := &cloudapp.WireOptions{
    Clock: fakeClock,
    Queue: sharedQueue,
}
cloud := cloudapp.Wire(cfg, cloudOpts)
```

## Repository Layout

```text
cmd/
  cloudapi/main.go          # Cloud API server binary
  onprem/main.go            # On-prem app binary
internal/
  app/
    cloud/                  # Cloud application wiring
      config.go             # Environment config loading
      wire.go               # Dependency composition
    onprem/                 # On-prem application wiring
      config.go             # Environment config loading
      wire.go               # Dependency composition
  core/
    domain/                 # Domain types (no IO)
      album.go              # Album entity
      album_video.go        # Manifest membership (AlbumVideo)
      video.go              # Video metadata
      object.go             # Stored object record
    services/               # Business logic, port interfaces
      clock.go              # Clock interface for testable time
      queue.go              # Queue port interface
      album_repository.go   # Album repository port
      album_video_repository.go  # Album video repository port
      video_repository.go   # Video repository port
      object_repository.go  # Object repository port
      staging_storage.go    # Staging storage port
      vault.go              # MediaVault port
      cloud_client.go       # Cloud client port
      user_albums.go        # UserAlbums service
      album_manifest_upload.go  # AlbumManifestUpload service
      video_upload.go       # VideoUpload service
      sync_user.go          # SyncUser consumer
      album_manifest_upload_consumer.go  # AlbumManifestUpload consumer
      video_upload_consumer.go # VideoUpload consumer
      eventual_consistency.go   # EC worker and check consumer
  adapters/
    queue/
      memory/               # In-memory queue implementation
    http/
      cloud/                # Cloud HTTP handlers
        user_albums_handler.go  # UserAlbumsHandler
        album_manifest_upload_handler.go  # AlbumManifestUploadHandler
        video_upload_handler.go  # VideoUploadHandler
      onprem/               # On-prem HTTP handlers
        cloud_client.go     # HTTP client for cloud API
        video_receiver.go   # Receives videos from MediaVault (VideoReceiver)
        video_sender.go     # Sends videos to receiver (VideoSender)
    mediavault/             # MediaVault adapter (JIT config reader)
      mediavault.go         # DatabaseScopedMediaVault implementation
      registry.go           # FileSystemMediaVaultRegistry implementation
      config.go             # Config types
    storage/
      fs/                   # Filesystem adapter for staging
    repo/
      memory/               # In-memory repository adapters
        album_repository.go         # AlbumRepository
        album_video_repository.go   # AlbumVideoRepository
        video_repository.go         # VideoRepository
        object_repository.go
      mysql/                # MySQL repository adapters (Milestone 6)
migrations/                 # SQL migrations (Milestone 6)
docker-compose.yml          # MySQL container (Milestone 6)
ARCHITECTURE.md             # This file
```

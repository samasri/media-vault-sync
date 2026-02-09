-- +migrate Up
CREATE TABLE IF NOT EXISTS albums (
    uid VARCHAR(255) NOT NULL PRIMARY KEY,
    provider_id VARCHAR(255) NOT NULL,
    database_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    album_uid VARCHAR(255) NOT NULL,
    synced BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_album (provider_id, database_id, album_uid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS album_videos (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    provider_id VARCHAR(255) NOT NULL,
    database_id VARCHAR(255) NOT NULL,
    album_uid VARCHAR(255) NOT NULL,
    video_uid VARCHAR(255) NOT NULL,
    UNIQUE KEY uk_album_video (provider_id, database_id, album_uid, video_uid),
    INDEX idx_album_lookup (provider_id, database_id, album_uid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS videos (
    uid VARCHAR(255) NOT NULL PRIMARY KEY,
    provider_id VARCHAR(255) NOT NULL,
    database_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    video_uid VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_video (provider_id, database_id, video_uid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS objects (
    uid VARCHAR(255) NOT NULL PRIMARY KEY,
    provider_id VARCHAR(255) NOT NULL,
    database_id VARCHAR(255) NOT NULL,
    video_uid VARCHAR(255) NOT NULL,
    storage_key VARCHAR(1024) NOT NULL,
    size_bytes BIGINT NOT NULL,
    checksum VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_object (provider_id, database_id, video_uid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +migrate Down
DROP TABLE IF EXISTS objects;
DROP TABLE IF EXISTS videos;
DROP TABLE IF EXISTS album_videos;
DROP TABLE IF EXISTS albums;

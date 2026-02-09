package cloud

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	RepoBackend      string
	MySQLDSN         string
	ScanInterval     time.Duration
	QueueTickInterval time.Duration
}

func LoadConfig() Config {
	cfg := Config{
		Port:             getEnv("CLOUD_PORT", "8080"),
		RepoBackend:      getEnv("REPO_BACKEND", "memory"),
		MySQLDSN:         getEnv("MYSQL_DSN", ""),
		ScanInterval:     getDurationEnv("SCAN_INTERVAL", 30*time.Second),
		QueueTickInterval: getDurationEnv("QUEUE_TICK_INTERVAL", 100*time.Millisecond),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	if ms, err := strconv.Atoi(val); err == nil {
		return time.Duration(ms) * time.Millisecond
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	return defaultVal
}

package onprem

import (
	"os"
	"time"
)

type Config struct {
	Port                 string
	MediaVaultConfigPath string
	StagingDir           string
	CloudBaseURL         string
	ProviderID           string
	QueueTickInterval    time.Duration
	ReceiverURL          string
}

func LoadConfig() Config {
	cfg := Config{
		Port:                 getEnv("ONPREM_PORT", "8081"),
		MediaVaultConfigPath: getEnv("MEDIAVAULT_CONFIG_PATH", "mediavault_config.json"),
		StagingDir:           getEnv("STAGING_DIR", "/tmp/staging"),
		CloudBaseURL:         getEnv("CLOUD_BASE_URL", "http://localhost:8080"),
		ProviderID:           getEnv("PROVIDER_ID", ""),
		QueueTickInterval:    getDurationEnv("QUEUE_TICK_INTERVAL", 100*time.Millisecond),
		ReceiverURL:          getEnv("RECEIVER_URL", ""),
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
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	return defaultVal
}

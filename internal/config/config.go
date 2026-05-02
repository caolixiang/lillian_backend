package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultServiceName           = "lillian-backend"
	DefaultPort                  = "8787"
	DefaultUpstreamTimeout       = 10 * time.Minute
	DefaultTaskPollInterval      = 2 * time.Second
	DefaultTaskWorkerConcurrency = 2
	DefaultAutoMigrate           = true
)

type Config struct {
	ServiceName           string
	Version               string
	Environment           string
	ListenAddr            string
	PublicAPIBaseURL      string
	CORSOrigin            string
	AdminToken            string
	LicenseKeyPepper      string
	ProviderSecret        string
	DatabaseURL           string
	Database              DatabaseConfig
	AutoMigrate           bool
	MigrationsDir         string
	UpstreamTimeout       time.Duration
	TaskPollInterval      time.Duration
	TaskWorkerConcurrency int
	Storage               StorageConfig
	EPUSDT                EPUSDTConfig
}

type DatabaseConfig struct {
	PoolMaxConns int
	PoolMinConns int
}

type StorageConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
	ForcePathStyle  bool
}

type EPUSDTConfig struct {
	BaseURL   string
	PID       string
	SecretKey string
	Currency  string
	Token     string
	Network   string
}

func Load(version string) (Config, error) {
	if version == "" {
		version = "dev"
	}

	host := stringEnv("HOST", "0.0.0.0")
	port := stringEnv("PORT", DefaultPort)
	listenAddr := host
	if !strings.Contains(host, ":") {
		listenAddr = host + ":" + port
	}

	cfg := Config{
		ServiceName:      stringEnv("SERVICE_NAME", DefaultServiceName),
		Version:          version,
		Environment:      stringEnv("APP_ENV", "local"),
		ListenAddr:       listenAddr,
		PublicAPIBaseURL: strings.TrimRight(stringEnv("PUBLIC_API_BASE_URL", ""), "/"),
		CORSOrigin:       stringEnv("CORS_ORIGIN", "*"),
		AdminToken:       stringEnv("ADMIN_TOKEN", ""),
		LicenseKeyPepper: stringEnv("LICENSE_KEY_PEPPER", ""),
		ProviderSecret:   stringEnv("PROVIDER_CREDENTIAL_SECRET", ""),
		DatabaseURL:      stringEnv("DATABASE_URL", ""),
		Database: DatabaseConfig{
			PoolMaxConns: intEnv("DB_POOL_MAX_CONNS", 0),
			PoolMinConns: intEnv("DB_POOL_MIN_CONNS", 0),
		},
		AutoMigrate:           boolEnv("AUTO_MIGRATE", DefaultAutoMigrate),
		MigrationsDir:         stringEnv("MIGRATIONS_DIR", "migrations"),
		UpstreamTimeout:       durationSecondsEnv("UPSTREAM_TIMEOUT_SECONDS", DefaultUpstreamTimeout),
		TaskPollInterval:      durationSecondsEnv("TASK_POLL_INTERVAL_SECONDS", DefaultTaskPollInterval),
		TaskWorkerConcurrency: intEnv("TASK_WORKER_CONCURRENCY", DefaultTaskWorkerConcurrency),
		Storage: StorageConfig{
			Endpoint:        strings.TrimRight(stringEnv("S3_ENDPOINT", ""), "/"),
			Region:          stringEnv("S3_REGION", "auto"),
			Bucket:          stringEnv("S3_BUCKET", ""),
			AccessKeyID:     stringEnv("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey: stringEnv("S3_SECRET_ACCESS_KEY", ""),
			PublicBaseURL:   strings.TrimRight(stringEnv("S3_PUBLIC_BASE_URL", ""), "/"),
			ForcePathStyle:  boolEnv("S3_FORCE_PATH_STYLE", true),
		},
		EPUSDT: EPUSDTConfig{
			BaseURL:   strings.TrimRight(stringEnv("EPUSDT_BASE_URL", ""), "/"),
			PID:       stringEnv("EPUSDT_PID", ""),
			SecretKey: stringEnv("EPUSDT_SECRET_KEY", ""),
			Currency:  stringEnv("EPUSDT_CURRENCY", "USDT"),
			Token:     stringEnv("EPUSDT_TOKEN", "USDT"),
			Network:   stringEnv("EPUSDT_NETWORK", "TRON"),
		},
	}

	if cfg.TaskWorkerConcurrency <= 0 {
		return Config{}, fmt.Errorf("TASK_WORKER_CONCURRENCY must be positive")
	}
	return cfg, nil
}

func stringEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func boolEnv(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func durationSecondsEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

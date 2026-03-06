package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	LogLevel    string
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:        getEnv("AMPULLA_HOST", "0.0.0.0"),
		Port:        getEnv("AMPULLA_PORT", "8090"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    getEnv("AMPULLA_LOG_LEVEL", "info"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func (c *Config) Addr() string {
	return c.Host + ":" + c.Port
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

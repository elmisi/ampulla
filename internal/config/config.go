package config

import (
	"crypto/rand"
	"fmt"
	"os"
)

type Config struct {
	Host            string
	Port            string
	DatabaseURL     string
	LogLevel        string
	AdminUser       string
	AdminPassword   string
	SessionSecret   []byte
	Domain          string
	FrontendSentryDSN string
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:          getEnv("AMPULLA_HOST", "0.0.0.0"),
		Port:          getEnv("AMPULLA_PORT", "8090"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		LogLevel:      getEnv("AMPULLA_LOG_LEVEL", "info"),
		AdminUser:     os.Getenv("ADMIN_USER"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		Domain:            getEnv("AMPULLA_DOMAIN", "ampulla.elmisi.com"),
		FrontendSentryDSN: os.Getenv("SENTRY_FRONTEND_DSN"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		cfg.SessionSecret = []byte(secret)
	} else {
		cfg.SessionSecret = make([]byte, 32)
		if _, err := rand.Read(cfg.SessionSecret); err != nil {
			return nil, fmt.Errorf("generate session secret: %w", err)
		}
	}

	return cfg, nil
}

func (c *Config) AdminEnabled() bool {
	return c.AdminUser != "" && c.AdminPassword != ""
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

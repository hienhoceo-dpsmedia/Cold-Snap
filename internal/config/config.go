package config

import (
    "fmt"
    "net/url"
    "os"
    "strconv"
)

type Config struct {
    Role        string
    APIPort     int
    DatabaseURL string
    RedisURL    string
    WorkerName  string
    WorkerVersion string
    AdminToken  string
    AdminUser   string
    AdminPass   string
    PublicURL   string
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func Parse() (*Config, error) {
    role := getenv("ROLE", "api")
    portStr := getenv("API_PORT", "8080")
    port, err := strconv.Atoi(portStr)
    if err != nil {
        return nil, fmt.Errorf("invalid API_PORT: %w", err)
    }
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        return nil, fmt.Errorf("DATABASE_URL is required")
    }
    if _, err := url.Parse(dbURL); err != nil {
        return nil, fmt.Errorf("invalid DATABASE_URL: %w", err)
    }
    redisURL := os.Getenv("REDIS_URL")
    if redisURL == "" {
        return nil, fmt.Errorf("REDIS_URL is required")
    }
    return &Config{
        Role:        role,
        APIPort:     port,
        DatabaseURL: dbURL,
        RedisURL:    redisURL,
        WorkerName:  getenv("WORKER_NAME", "worker-1"),
        WorkerVersion: getenv("WORKER_VERSION", "v0.1.0"),
        AdminToken:  getenv("ADMIN_TOKEN", ""),
        AdminUser:   getenv("ADMIN_USER", ""),
        AdminPass:   getenv("ADMIN_PASS", ""),
        PublicURL:   getenv("PUBLIC_URL", ""),
    }, nil
}

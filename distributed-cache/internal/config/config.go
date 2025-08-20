package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds all configuration for the cache server
type Config struct {
    Server    ServerConfig    `json:"server"`
    Cache     CacheConfig     `json:"cache"`
    Cluster   ClusterConfig   `json:"cluster"`
    Logging   LoggingConfig   `json:"logging"`
    Metrics   MetricsConfig   `json:"metrics"`
}

type ServerConfig struct {
    Host            string        `json:"host"`
    Port            int           `json:"port"`
    ReadTimeout     time.Duration `json:"read_timeout"`
    WriteTimeout    time.Duration `json:"write_timeout"`
    ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

type CacheConfig struct {
    MaxSize        int64         `json:"max_size"`
    DefaultTTL     time.Duration `json:"default_ttl"`
    CleanupInterval time.Duration `json:"cleanup_interval"`
    EvictionPolicy string        `json:"eviction_policy"`
}

type ClusterConfig struct {
    NodeID          string   `json:"node_id"`
    Seeds           []string `json:"seeds"`
    HeartbeatInterval time.Duration `json:"heartbeat_interval"`
    ElectionTimeout time.Duration `json:"election_timeout"`
    ReplicationFactor int    `json:"replication_factor"`
}

type LoggingConfig struct {
    Level  string `json:"level"`
    Format string `json:"format"`
}

type MetricsConfig struct {
    Enabled bool   `json:"enabled"`
    Port    int    `json:"port"`
    Path    string `json:"path"`
}

// LoadConfig loads configuration from file with defaults
func LoadConfig(configPath string) (*Config, error) {
    config := &Config{
        Server: ServerConfig{
            Host:            "localhost",
            Port:            8080,
            ReadTimeout:     30 * time.Second,
            WriteTimeout:    30 * time.Second,
            ShutdownTimeout: 10 * time.Second,
        },
        Cache: CacheConfig{
            MaxSize:         1000000, // 1M entries
            DefaultTTL:      time.Hour,
            CleanupInterval: 5 * time.Minute,
            EvictionPolicy:  "lru",
        },
        Cluster: ClusterConfig{
            NodeID:            generateNodeID(),
            Seeds:             []string{},
            HeartbeatInterval: 1 * time.Second,
            ElectionTimeout:   5 * time.Second,
            ReplicationFactor: 3,
        },
        Logging: LoggingConfig{
            Level:  "info",
            Format: "json",
        },
        Metrics: MetricsConfig{
            Enabled: true,
            Port:    9090,
            Path:    "/metrics",
        },
    }

    if configPath != "" {
        if err := loadFromFile(config, configPath); err != nil {
            return nil, fmt.Errorf("failed to load config from file: %w", err)
        }
    }

    // Override with environment variables
    loadFromEnv(config)

    return config, nil
}

func loadFromFile(config *Config, path string) error {
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer file.Close()

    decoder := json.NewDecoder(file)
    return decoder.Decode(config)
}

func loadFromEnv(config *Config) {
    if host := os.Getenv("CACHE_HOST"); host != "" {
        config.Server.Host = host
    }
    if port := os.Getenv("CACHE_PORT"); port != "" {
        // Parse port and set it
    }
    // Add more environment variable overrides as needed
}

func generateNodeID() string {
    hostname, _ := os.Hostname()
    return fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
}
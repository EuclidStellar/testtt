package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/yourusername/distributed-cache/internal/cache"
	"github.com/yourusername/distributed-cache/internal/config"
	"github.com/yourusername/distributed-cache/pkg/types"
)

type Server struct {
    cache  types.Cache
    config *config.Config
    server *http.Server
}

func NewServer(cache types.Cache, cfg *config.Config) *Server {
    return &Server{
        cache:  cache,
        config: cfg,
    }
}

func (s *Server) Start(ctx context.Context) error {
    mux := http.NewServeMux()
    
    // Cache endpoints
    mux.HandleFunc("/cache/", s.handleCache)
    mux.HandleFunc("/stats", s.handleStats)
    mux.HandleFunc("/health", s.handleHealth)

    s.server = &http.Server{
        Addr:         fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
        Handler:      mux,
        ReadTimeout:  s.config.Server.ReadTimeout,
        WriteTimeout: s.config.Server.WriteTimeout,
    }

    fmt.Printf("Starting cache server on %s\n", s.server.Addr)
    return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    return s.server.Shutdown(ctx)
}

func (s *Server) handleCache(w http.ResponseWriter, r *http.Request) {
    key := r.URL.Path[7:] // Remove "/cache/" prefix
    if key == "" {
        http.Error(w, "Key is required", http.StatusBadRequest)
        return
    }

    ctx := r.Context()

    switch r.Method {
    case http.MethodGet:
        entry, err := s.cache.Get(ctx, key)
        if err != nil {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(entry)

    case http.MethodPut, http.MethodPost:
        body, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, "Failed to read body", http.StatusBadRequest)
            return
        }

        ttl := s.config.Cache.DefaultTTL
        if ttlStr := r.Header.Get("X-TTL"); ttlStr != "" {
            if ttlSeconds, err := strconv.Atoi(ttlStr); err == nil {
                ttl = time.Duration(ttlSeconds) * time.Second
            }
        }

        if err := s.cache.Set(ctx, key, body, ttl); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusCreated)

    case http.MethodDelete:
        if err := s.cache.Delete(ctx, key); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusNoContent)

    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    if memCache, ok := s.cache.(*cache.MemoryStorage); ok {
        stats := memCache.GetStats()
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(stats)
    } else {
        http.Error(w, "Stats not available", http.StatusNotImplemented)
    }
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    health := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now(),
        "size":      s.cache.Size(),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
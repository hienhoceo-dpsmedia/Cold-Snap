package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/redis/go-redis/v9"

    "cold-snap/internal/config"
    "cold-snap/internal/db"
    "cold-snap/internal/ingest"
    "cold-snap/internal/migrate"
    "cold-snap/internal/worker"
)

func main() {
    cfg, err := config.Parse()
    if err != nil { log.Fatalf("config: %v", err) }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    logger := func(msg string, kv ...any) {
        ts := time.Now().Format(time.RFC3339)
        fmt.Println(ts, msg, kv)
    }

    database, err := db.Connect(ctx, cfg.DatabaseURL)
    if err != nil { log.Fatalf("db connect: %v", err) }
    defer database.Close()

    if err := migrate.Apply(ctx, database.Pool); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    rurl, err := url.Parse(cfg.RedisURL)
    if err != nil { log.Fatalf("redis url: %v", err) }
    _ = rurl
    redisOpts, err := redis.ParseURL(cfg.RedisURL)
    if err != nil { log.Fatalf("redis opts: %v", err) }
    rdb := redis.NewClient(redisOpts)
    if err := rdb.Ping(ctx).Err(); err != nil { log.Fatalf("redis ping: %v", err) }

    switch cfg.Role {
    case "api":
        mux := http.NewServeMux()
        srv := ingest.NewServer(database.Pool, logger, cfg.AdminToken)
        srv.AdminUser = cfg.AdminUser
        srv.AdminPass = cfg.AdminPass
        srv.Routes(mux)
        addr := fmt.Sprintf(":%d", cfg.APIPort)
        logger("api_listen", "addr", addr)
        srv := &http.Server{Addr: addr, Handler: mux}
        go func() {
            <-ctx.Done()
            _ = srv.Shutdown(context.Background())
        }()
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %v", err)
        }
    case "worker":
        wk := worker.New(database.Pool, rdb, cfg.WorkerName, cfg.WorkerVersion, logger)
        logger("worker_start", "name", cfg.WorkerName)
        if err := wk.Run(ctx); err != nil && ctx.Err() == nil {
            log.Fatalf("worker: %v", err)
        }
    default:
        fmt.Println("unknown ROLE", cfg.Role)
        os.Exit(1)
    }
}

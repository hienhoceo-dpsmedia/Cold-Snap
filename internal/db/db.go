package db

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
    Pool *pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*DB, error) {
    cfg, err := pgxpool.ParseConfig(url)
    if err != nil {
        return nil, fmt.Errorf("parse db url: %w", err)
    }
    cfg.MaxConns = 10
    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("pgxpool: %w", err)
    }
    ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    if err := pool.Ping(ctxPing); err != nil {
        pool.Close()
        return nil, fmt.Errorf("db ping: %w", err)
    }
    return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
    d.Pool.Close()
}


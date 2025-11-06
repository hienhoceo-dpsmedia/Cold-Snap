package migrate

import (
    "context"
    "embed"
    "fmt"
    "sort"
    "strings"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
    if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
        return fmt.Errorf("create schema_migrations: %w", err)
    }

    entries, err := migrationsFS.ReadDir("migrations")
    if err != nil {
        return fmt.Errorf("read migrations: %w", err)
    }
    names := make([]string, 0, len(entries))
    for _, e := range entries {
        if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
            names = append(names, e.Name())
        }
    }
    sort.Strings(names)

    for _, name := range names {
        var exists bool
        if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
            return fmt.Errorf("check migration %s: %w", name, err)
        }
        if exists {
            continue
        }
        sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
        if err != nil {
            return fmt.Errorf("read migration %s: %w", name, err)
        }
        tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
        if err != nil {
            return fmt.Errorf("begin tx: %w", err)
        }
        if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
            _ = tx.Rollback(ctx)
            return fmt.Errorf("apply migration %s: %w", name, err)
        }
        if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
            _ = tx.Rollback(ctx)
            return fmt.Errorf("record migration %s: %w", name, err)
        }
        if err := tx.Commit(ctx); err != nil {
            return fmt.Errorf("commit migration %s: %w", name, err)
        }
    }
    return nil
}

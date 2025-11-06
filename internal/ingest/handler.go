package ingest

import (
    "context"
    "crypto/sha256"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net"
    "net/http"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
    DB     *pgxpool.Pool
    Logger func(msg string, kv ...any)
}

func NewServer(db *pgxpool.Pool, logger func(string, ...any)) *Server {
    return &Server{DB: db, Logger: logger}
}

func (s *Server) Routes(mux *http.ServeMux) {
    mux.HandleFunc("/ingest", s.handleIngest)
    mux.HandleFunc("/ingest/", s.handleIngestPath)
    mux.HandleFunc("/events/", s.handleEvents)
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
}

func (s *Server) handleIngestPath(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ingest/"), "/")
    if len(parts) < 1 || parts[0] == "" {
        http.Error(w, "missing token", http.StatusUnauthorized)
        return
    }
    token := parts[0]
    s.handleIngestWithToken(w, r, token)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
        token := strings.TrimSpace(auth[7:])
        s.handleIngestWithToken(w, r, token)
        return
    }
    http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// handleEvents implements simple admin actions like replay.
// POST /events/{id}/replay
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    path := strings.TrimPrefix(r.URL.Path, "/events/")
    parts := strings.Split(path, "/")
    if len(parts) < 2 || parts[1] != "replay" {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    eventID := parts[0]
    ctx := r.Context()
    // Ensure event exists and load minimal context
    var contentType *string
    err := s.DB.QueryRow(ctx, `SELECT content_type FROM event WHERE event_id=$1::uuid`, eventID).Scan(&contentType)
    if err != nil {
        http.Error(w, "event not found", http.StatusNotFound)
        return
    }
    // Insert attempts for current enabled routes with simple filter
    ct := ""
    if contentType != nil { ct = *contentType }
    cmd, err := s.DB.Exec(ctx, `
        INSERT INTO delivery_attempt (event_id, route_id, attempt_no, status, next_at)
        SELECT $1::uuid, r.route_id, 0, 'pending', now()
        FROM route r
        JOIN event e ON e.source_id = r.source_id
        WHERE e.event_id = $1::uuid
          AND r.enabled = true
          AND (r.content_type_like IS NULL OR $2 LIKE r.content_type_like)
    `, eventID, ct)
    if err != nil {
        s.Logger("replay_insert_error", "error", err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]any{
        "event_id": eventID,
        "attempts_created": cmd.RowsAffected(),
    })
}

func (s *Server) handleIngestWithToken(w http.ResponseWriter, r *http.Request, token string) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    ctx := r.Context()
    src, err := s.lookupSource(ctx, token)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        s.Logger("source_lookup_error", "error", err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    if !src.Enabled {
        http.Error(w, "source disabled", http.StatusForbidden)
        return
    }
    // IP allowlist check
    if len(src.IPAllowCIDRs) > 0 {
        ip, _, _ := net.SplitHostPort(r.RemoteAddr)
        if ip == "" {
            ip = r.Header.Get("X-Forwarded-For")
            if idx := strings.Index(ip, ","); idx > 0 { ip = strings.TrimSpace(ip[:idx]) }
        }
        if ip != "" && !ipAllowed(ip, src.IPAllowCIDRs) {
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }
    }
    // Body size limit
    r.Body = http.MaxBytesReader(w, r.Body, int64(src.MaxBodyBytes))
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "invalid body", http.StatusBadRequest)
        return
    }
    contentType := r.Header.Get("Content-Type")
    hdrs := canonicalHeaders(r.Header)
    hdrJSON, _ := json.Marshal(hdrs)
    idk := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
    var idempotencyKey *string
    if idk != "" { idempotencyKey = &idk }
    sum := sha256.Sum256(body)

    // Derive path after /ingest/{token}
    relPath := "/"
    wantPrefix := "/ingest/" + token
    if strings.HasPrefix(r.URL.Path, wantPrefix) {
        tail := strings.TrimPrefix(r.URL.Path, wantPrefix)
        if tail == "" { relPath = "/" } else { relPath = tail }
    }

    // Idempotency check
    var existingIDStr *string
    if idempotencyKey != nil {
        row := s.DB.QueryRow(ctx, `SELECT event_id::text FROM event WHERE source_id=(SELECT source_id FROM source WHERE token=$1) AND idempotency_key=$2`, token, *idempotencyKey)
        var eid string
        if err := row.Scan(&eid); err == nil {
            existingIDStr = &eid
        }
    }

    var eventIDStr string
    if existingIDStr != nil {
        eventIDStr = *existingIDStr
    } else {
        ipStr := clientIP(r)
        now := time.Now()
        err = s.DB.QueryRow(ctx, `
            INSERT INTO event (source_id, received_at, content_type, headers, body, body_size, source_ip, idempotency_key, body_hash_sha256, method, path, query)
            VALUES ((SELECT source_id FROM source WHERE token=$1), $2, $3, $4::jsonb, $5, $6, $7::inet, $8, $9, $10, $11, $12)
            RETURNING event_id::text
        `,
            token, now, nullableString(contentType), string(hdrJSON), body, len(body), nullableString(ipStr), idempotencyKey, sum[:], r.Method, relPath, r.URL.RawQuery,
        ).Scan(&eventIDStr)
        if err != nil {
            s.Logger("event_insert_error", "error", err)
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
        // create attempts for enabled routes (MVP: only content_type_like)
        _, err = s.DB.Exec(ctx, `
            INSERT INTO delivery_attempt (event_id, route_id, attempt_no, status, next_at)
            SELECT $1, r.route_id, 0, 'pending', now()
            FROM route r
            JOIN source s ON s.source_id = r.source_id
            WHERE s.token=$2 AND r.enabled=true
              AND (r.content_type_like IS NULL OR $3 LIKE r.content_type_like)
        `, eventID, token, contentType)
        if err != nil {
            s.Logger("attempts_insert_error", "error", err)
            http.Error(w, "internal error", http.StatusInternalServerError)
            return
        }
    }

    var count int
    _ = s.DB.QueryRow(ctx, `SELECT count(*) FROM delivery_attempt WHERE event_id=$1::uuid`, eventIDStr).Scan(&count)
    resp := map[string]any{"event_id": eventIDStr, "attempts_created": count}
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusAccepted)
    _ = json.NewEncoder(w).Encode(resp)
}

type sourceRow struct {
    SourceID string
    Enabled  bool
    IPAllowCIDRs []string
    MaxBodyBytes int32
}

func (s *Server) lookupSource(ctx context.Context, token string) (*sourceRow, error) {
    row := s.DB.QueryRow(ctx, `SELECT source_id, enabled, coalesce(ip_allow_cidrs,'{}')::text[], max_body_bytes FROM source WHERE token=$1`, token)
    var sr sourceRow
    if err := row.Scan(&sr.SourceID, &sr.Enabled, &sr.IPAllowCIDRs, &sr.MaxBodyBytes); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, sql.ErrNoRows
        }
        return nil, err
    }
    return &sr, nil
}

func canonicalHeaders(h http.Header) map[string]string {
    out := make(map[string]string, len(h))
    for k, v := range h {
        lk := strings.ToLower(k)
        out[lk] = strings.Join(v, ", ")
    }
    return out
}

func clientIP(r *http.Request) string {
    xff := r.Header.Get("X-Forwarded-For")
    if xff != "" {
        parts := strings.Split(xff, ",")
        if len(parts) > 0 {
            return strings.TrimSpace(parts[0])
        }
    }
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err == nil {
        return ip
    }
    return ""
}

func ipAllowed(ip string, cidrs []string) bool {
    pip := net.ParseIP(ip)
    if pip == nil {
        return false
    }
    for _, c := range cidrs {
        _, n, err := net.ParseCIDR(c)
        if err != nil {
            continue
        }
        if n.Contains(pip) {
            return true
        }
    }
    return false
}

func nullableString(s string) *string {
    if s == "" {
        return nil
    }
    return &s
}

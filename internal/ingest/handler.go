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
    "strconv"
    "strings"
    "time"

    "github.com/gofrs/uuid/v5"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    adminui "cold-snap/internal/admin"
)

type Server struct {
    DB         *pgxpool.Pool
    Logger     func(msg string, kv ...any)
    AdminToken string
    AdminUser  string
    AdminPass  string
}

func NewServer(db *pgxpool.Pool, logger func(string, ...any), adminToken string) *Server {
    return &Server{DB: db, Logger: logger, AdminToken: adminToken}
}

func (s *Server) Routes(mux *http.ServeMux) {
    mux.HandleFunc("/ingest", s.handleIngest)
    mux.HandleFunc("/ingest/", s.handleIngestPath)
    mux.HandleFunc("/events/", s.handleEvents)
    if s.AdminToken != "" {
        mux.HandleFunc("/admin/sources", s.adminSources)
        mux.HandleFunc("/admin/sources/", s.adminSourcesSub)
        mux.HandleFunc("/admin/destinations", s.adminDestinations)
        mux.HandleFunc("/admin/routes", s.adminRoutes)
        mux.HandleFunc("/admin/routes/", s.adminRoutesSub)
        mux.HandleFunc("/admin/events", s.adminEvents)
        mux.HandleFunc("/admin/attempts", s.adminAttempts)
        // Static admin console; require Basic auth if ADMIN_USER/ADMIN_PASS are set
        h := http.StripPrefix("/console/", adminHandler())
        if s.AdminUser != "" {
            h = basicAuth(h, s.AdminUser, s.AdminPass)
        }
        mux.Handle("/console/", h)
    }
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
}

func adminHandler() http.Handler { return adminui.Handler() }

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

// --- Admin REST (bearer-protected) ---
func (s *Server) checkAdmin(w http.ResponseWriter, r *http.Request) bool {
    if s.AdminToken == "" && s.AdminUser == "" { return false }
    auth := r.Header.Get("Authorization")
    low := strings.ToLower(auth)
    if strings.HasPrefix(low, "bearer ") && s.AdminToken != "" {
        tok := strings.TrimSpace(auth[7:])
        if tok == s.AdminToken { return true }
    }
    if strings.HasPrefix(low, "basic ") && s.AdminUser != "" {
        user, pass, ok := r.BasicAuth()
        if ok && user == s.AdminUser && pass == s.AdminPass { return true }
        w.Header().Set("WWW-Authenticate", "Basic realm=\"ColdSnap Admin\"")
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return false
    }
    if s.AdminToken != "" {
        w.Header().Set("WWW-Authenticate", "Bearer")
    }
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return false
}

func basicAuth(next http.Handler, user, pass string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        u, p, ok := r.BasicAuth()
        if !ok || u != user || p != pass {
            w.Header().Set("WWW-Authenticate", "Basic realm=\"ColdSnap Admin\"")
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// POST/GET /admin/sources
func (s *Server) adminSources(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    ctx := r.Context()
    switch r.Method {
    case http.MethodGet:
        rows, err := s.DB.Query(ctx, `SELECT source_id::text, name, enabled, max_body_bytes, coalesce(ip_allow_cidrs,'{}')::text[] FROM source ORDER BY created_at DESC`)
        if err != nil { http.Error(w, "internal", 500); return }
        defer rows.Close()
        var items []map[string]any
        for rows.Next() {
            var id, name string
            var enabled bool
            var maxBody int32
            var cidrs []string
            if err := rows.Scan(&id, &name, &enabled, &maxBody, &cidrs); err != nil { http.Error(w, "internal", 500); return }
            items = append(items, map[string]any{"source_id": id, "name": name, "enabled": enabled, "max_body_bytes": maxBody, "ip_allow_cidrs": cidrs})
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"items": items})
    case http.MethodPost:
        var req struct {
            Name         string    `json:"name"`
            Token        *string   `json:"token"`
            Enabled      *bool     `json:"enabled"`
            IPAllowCIDRs []string  `json:"ip_allow_cidrs"`
            MaxBodyBytes *int      `json:"max_body_bytes"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, "bad json", 400); return }
        if req.Name == "" { http.Error(w, "name required", 400); return }
        tok := ""
        if req.Token != nil && *req.Token != "" { tok = *req.Token } else { tok = uuid.Must(uuid.NewV4()).String() }
        enabled := true
        if req.Enabled != nil { enabled = *req.Enabled }
        maxBody := 1048576
        if req.MaxBodyBytes != nil { maxBody = *req.MaxBodyBytes }
        _, err := s.DB.Exec(ctx, `
            INSERT INTO source(name, token, enabled, ip_allow_cidrs, max_body_bytes)
            VALUES($1,$2,$3,$4,$5)
        `, req.Name, tok, enabled, req.IPAllowCIDRs, maxBody)
        if err != nil { s.Logger("admin_create_source_err", "err", err); http.Error(w, "conflict or error", 400); return }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"name": req.Name, "token": tok, "enabled": enabled})
    default:
        http.Error(w, "method not allowed", 405)
    }
}

// POST/GET /admin/destinations
func (s *Server) adminDestinations(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    ctx := r.Context()
    switch r.Method {
    case http.MethodGet:
        rows, err := s.DB.Query(ctx, `SELECT destination_id::text, name, url, headers, secret, connect_timeout_s, timeout_s, verify_tls, max_rps, burst, max_inflight, append_path FROM destination ORDER BY created_at DESC`)
        if err != nil { http.Error(w, "internal", 500); return }
        defer rows.Close()
        var items []map[string]any
        for rows.Next() {
            var id, name, urlStr string
            var headersJSON []byte
            var secret *string
            var cto, to int32
            var verify bool
            var rps float64
            var burst, inflight int32
            var appendPath bool
            if err := rows.Scan(&id, &name, &urlStr, &headersJSON, &secret, &cto, &to, &verify, &rps, &burst, &inflight, &appendPath); err != nil { http.Error(w, "internal", 500); return }
            var headers map[string]string
            _ = json.Unmarshal(headersJSON, &headers)
            items = append(items, map[string]any{"destination_id": id, "name": name, "url": urlStr, "headers": headers, "connect_timeout_s": cto, "timeout_s": to, "verify_tls": verify, "max_rps": rps, "burst": burst, "max_inflight": inflight, "append_path": appendPath})
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"items": items})
    case http.MethodPost:
        var req struct {
            Name   string             `json:"name"`
            URL    string             `json:"url"`
            Headers map[string]string `json:"headers"`
            Secret *string            `json:"secret"`
            ConnectTimeoutS *int      `json:"connect_timeout_s"`
            TimeoutS *int             `json:"timeout_s"`
            VerifyTLS *bool           `json:"verify_tls"`
            MaxRPS *float64           `json:"max_rps"`
            Burst *int                `json:"burst"`
            MaxInflight *int          `json:"max_inflight"`
            AppendPath *bool          `json:"append_path"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, "bad json", 400); return }
        if req.Name == "" || req.URL == "" { http.Error(w, "name and url required", 400); return }
        hdrJSON, _ := json.Marshal(req.Headers)
        cto := 5; if req.ConnectTimeoutS != nil { cto = *req.ConnectTimeoutS }
        to := 15; if req.TimeoutS != nil { to = *req.TimeoutS }
        verify := true; if req.VerifyTLS != nil { verify = *req.VerifyTLS }
        rps := 5.0; if req.MaxRPS != nil { rps = *req.MaxRPS }
        burst := 10; if req.Burst != nil { burst = *req.Burst }
        inflight := 5; if req.MaxInflight != nil { inflight = *req.MaxInflight }
        appendPath := false; if req.AppendPath != nil { appendPath = *req.AppendPath }
        _, err := s.DB.Exec(ctx, `
            INSERT INTO destination(name, url, headers, secret, connect_timeout_s, timeout_s, verify_tls, max_rps, burst, max_inflight, append_path)
            VALUES($1,$2,$3::jsonb,$4,$5,$6,$7,$8,$9,$10,$11)
        `, req.Name, req.URL, string(hdrJSON), req.Secret, cto, to, verify, rps, burst, inflight, appendPath)
        if err != nil { s.Logger("admin_create_dest_err", "err", err); http.Error(w, "conflict or error", 400); return }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"name": req.Name, "url": req.URL})
    default:
        http.Error(w, "method not allowed", 405)
    }
}

// POST/GET /admin/routes
func (s *Server) adminRoutes(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    ctx := r.Context()
    switch r.Method {
    case http.MethodGet:
        rows, err := s.DB.Query(ctx, `
            SELECT r.route_id::text, r.enabled, r.content_type_like, r.ord,
                   s.source_id::text, s.name, d.destination_id::text, d.name
            FROM route r JOIN source s ON s.source_id=r.source_id JOIN destination d ON d.destination_id=r.destination_id
            ORDER BY s.name, d.name
        `)
        if err != nil { http.Error(w, "internal", 500); return }
        defer rows.Close()
        var items []map[string]any
        for rows.Next() {
            var rid, sid, sname, did, dname string
            var enabled bool
            var ctLike *string
            var ord int16
            if err := rows.Scan(&rid, &enabled, &ctLike, &ord, &sid, &sname, &did, &dname); err != nil { http.Error(w, "internal", 500); return }
            items = append(items, map[string]any{"route_id": rid, "enabled": enabled, "content_type_like": ctLike, "ord": ord, "source": map[string]any{"id": sid, "name": sname}, "destination": map[string]any{"id": did, "name": dname}})
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"items": items})
    case http.MethodPost:
        var req struct {
            SourceName *string `json:"source_name"`
            SourceID   *string `json:"source_id"`
            DestinationName *string `json:"destination_name"`
            DestinationID   *string `json:"destination_id"`
            Enabled *bool   `json:"enabled"`
            ContentTypeLike *string `json:"content_type_like"`
            Ord *int        `json:"ord"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { http.Error(w, "bad json", 400); return }
        // Resolve IDs by name if needed
        sid := ""
        did := ""
        if req.SourceID != nil && *req.SourceID != "" { sid = *req.SourceID }
        if sid == "" && req.SourceName != nil {
            if err := s.DB.QueryRow(ctx, `SELECT source_id::text FROM source WHERE name=$1`, *req.SourceName).Scan(&sid); err != nil { http.Error(w, "unknown source", 400); return }
        }
        if req.DestinationID != nil && *req.DestinationID != "" { did = *req.DestinationID }
        if did == "" && req.DestinationName != nil {
            if err := s.DB.QueryRow(ctx, `SELECT destination_id::text FROM destination WHERE name=$1`, *req.DestinationName).Scan(&did); err != nil { http.Error(w, "unknown destination", 400); return }
        }
        if sid == "" || did == "" { http.Error(w, "source and destination required", 400); return }
        enabled := true; if req.Enabled != nil { enabled = *req.Enabled }
        ord := 0; if req.Ord != nil { ord = *req.Ord }
        _, err := s.DB.Exec(ctx, `
            INSERT INTO route(source_id, destination_id, enabled, content_type_like, ord)
            VALUES($1::uuid,$2::uuid,$3,$4,$5)
        `, sid, did, enabled, req.ContentTypeLike, ord)
        if err != nil { s.Logger("admin_create_route_err", "err", err); http.Error(w, "conflict or error", 400); return }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
    default:
        http.Error(w, "method not allowed", 405)
    }
}

// /admin/routes/{id}/pause or /resume
func (s *Server) adminRoutesSub(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/routes/"), "/")
    if len(parts) != 2 { http.Error(w, "not found", 404); return }
    routeID := parts[0]
    action := parts[1]
    switch action {
    case "pause":
        if _, err := s.DB.Exec(r.Context(), `UPDATE route SET enabled=false WHERE route_id=$1::uuid`, routeID); err != nil { http.Error(w, "internal", 500); return }
    case "resume":
        if _, err := s.DB.Exec(r.Context(), `UPDATE route SET enabled=true WHERE route_id=$1::uuid`, routeID); err != nil { http.Error(w, "internal", 500); return }
    default:
        http.Error(w, "not found", 404); return
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// /admin/sources/{id}/token (GET) or /rotate (POST)
func (s *Server) adminSourcesSub(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/sources/"), "/")
    if len(parts) < 1 { http.Error(w, "not found", 404); return }
    srcID := parts[0]
    if len(parts) == 1 {
        if r.Method != http.MethodGet { http.Error(w, "method not allowed", 405); return }
        var token string
        if err := s.DB.QueryRow(r.Context(), `SELECT token FROM source WHERE source_id=$1::uuid`, srcID).Scan(&token); err != nil { http.Error(w, "not found", 404); return }
        _ = json.NewEncoder(w).Encode(map[string]any{"token": token})
        return
    }
    if parts[1] == "rotate" && r.Method == http.MethodPost {
        newTok := uuid.Must(uuid.NewV4()).String()
        if _, err := s.DB.Exec(r.Context(), `UPDATE source SET token=$2 WHERE source_id=$1::uuid`, srcID, newTok); err != nil { http.Error(w, "internal", 500); return }
        _ = json.NewEncoder(w).Encode(map[string]any{"token": newTok})
        return
    }
    http.Error(w, "not found", 404)
}

// GET /admin/events?source_id=... or source_name=...&limit=20
func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    if r.Method != http.MethodGet { http.Error(w, "method not allowed", 405); return }
    q := r.URL.Query()
    sid := q.Get("source_id")
    sname := q.Get("source_name")
    limit := 20
    if v := q.Get("limit"); v != "" { if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 { limit = n } }
    if sid == "" && sname != "" {
        _ = s.DB.QueryRow(r.Context(), `SELECT source_id::text FROM source WHERE name=$1`, sname).Scan(&sid)
    }
    if sid == "" { http.Error(w, "source_id or source_name required", 400); return }
    rows, err := s.DB.Query(r.Context(), `
        SELECT event_id::text, received_at, coalesce(content_type,''), body_size, coalesce(method,''), coalesce(path,''), coalesce(query,'')
        FROM event WHERE source_id=$1::uuid ORDER BY received_at DESC LIMIT $2
    `, sid, limit)
    if err != nil { http.Error(w, "internal", 500); return }
    defer rows.Close()
    type item struct {
        EventID string    `json:"event_id"`
        ReceivedAt time.Time `json:"received_at"`
        ContentType string `json:"content_type"`
        BodySize int32 `json:"body_size"`
        Method string `json:"method"`
        Path string `json:"path"`
        Query string `json:"query"`
    }
    var out []item
    for rows.Next() {
        var it item
        if err := rows.Scan(&it.EventID, &it.ReceivedAt, &it.ContentType, &it.BodySize, &it.Method, &it.Path, &it.Query); err != nil { http.Error(w, "internal", 500); return }
        out = append(out, it)
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

// GET /admin/attempts?event_id=...&limit=50
func (s *Server) adminAttempts(w http.ResponseWriter, r *http.Request) {
    if !s.checkAdmin(w, r) { return }
    if r.Method != http.MethodGet { http.Error(w, "method not allowed", 405); return }
    q := r.URL.Query()
    eid := q.Get("event_id")
    if eid == "" { http.Error(w, "event_id required", 400); return }
    limit := 50
    if v := q.Get("limit"); v != "" { if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 { limit = n } }
    rows, err := s.DB.Query(r.Context(), `
        SELECT attempt_id::text, route_id::text, attempt_no, status::text, next_at, picked_at, succeeded_at, failed_at, http_code
        FROM delivery_attempt WHERE event_id=$1::uuid ORDER BY created_at ASC LIMIT $2
    `, eid, limit)
    if err != nil { http.Error(w, "internal", 500); return }
    defer rows.Close()
    type item struct {
        AttemptID string `json:"attempt_id"`
        RouteID string `json:"route_id"`
        AttemptNo int32 `json:"attempt_no"`
        Status string `json:"status"`
        NextAt *time.Time `json:"next_at"`
        PickedAt *time.Time `json:"picked_at"`
        SucceededAt *time.Time `json:"succeeded_at"`
        FailedAt *time.Time `json:"failed_at"`
        HTTPCode *int `json:"http_code"`
    }
    var out []item
    for rows.Next() {
        var it item
        if err := rows.Scan(&it.AttemptID, &it.RouteID, &it.AttemptNo, &it.Status, &it.NextAt, &it.PickedAt, &it.SucceededAt, &it.FailedAt, &it.HTTPCode); err != nil { http.Error(w, "internal", 500); return }
        out = append(out, it)
    }
    _ = json.NewEncoder(w).Encode(map[string]any{"items": out})
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

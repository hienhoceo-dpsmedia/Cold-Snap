package worker

import (
    "bytes"
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "crypto/tls"
    "errors"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "net/netip"
    "net/url"
    "os"
    "strings"
    "time"
    "strconv"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"

    "cold-snap/internal/redisrl"
    "cold-snap/internal/ssrf"
)

type Worker struct {
    DB     *pgxpool.Pool
    Rdb    *redis.Client
    Logger func(string, ...any)
    Name   string
    Version string
}

func New(db *pgxpool.Pool, rdb *redis.Client, name, version string, logger func(string, ...any)) *Worker {
    return &Worker{DB: db, Rdb: rdb, Name: name, Version: version, Logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
    limiter := redisrl.New(w.Rdb)
    // Start housekeeping in background
    go w.housekeeping(ctx)
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        attempt, ok, err := w.pickNext(ctx)
        if err != nil {
            w.Logger("pick_error", "error", err)
            time.Sleep(2 * time.Second)
            continue
        }
        if !ok {
            time.Sleep(1500 * time.Millisecond)
            continue
        }

        // destination details
        dest, routeID, event, err := w.loadAttemptContext(ctx, attempt.AttemptID)
        if err != nil {
            w.Logger("load_ctx_error", "attempt", attempt.AttemptID, "error", err)
            continue
        }

        // Circuit breaker open? (MVP defer)
        var openUntil *time.Time
        _ = w.DB.QueryRow(ctx, `SELECT open_until FROM destination_health WHERE destination_id=$1`, dest.DestinationID).Scan(&openUntil)
        if openUntil != nil && openUntil.After(time.Now()) {
            _, _ = w.DB.Exec(ctx, `UPDATE delivery_attempt SET status='pending', next_at=$2 WHERE attempt_id=$1`, attempt.AttemptID, *openUntil)
            continue
        }

        // rate limit check
        allowed, wait, err := limiter.Allow(ctx, dest.DestinationID, int(dest.Burst), dest.MaxRPS, int(dest.MaxInflight))
        if err != nil {
            w.Logger("rl_error", "error", err)
            // defer a bit
            _, _ = w.DB.Exec(ctx, `UPDATE delivery_attempt SET status='pending', next_at=now()+interval '1 second' WHERE attempt_id=$1`, attempt.AttemptID)
            continue
        }
        if !allowed {
            _, _ = w.DB.Exec(ctx, `UPDATE delivery_attempt SET status='pending', next_at=now()+($2::bigint||' milliseconds')::interval WHERE attempt_id=$1`, attempt.AttemptID, wait)
            continue
        }

        start := time.Now()
        code, respHdrs, respBody, sendErr := w.deliver(ctx, dest, event)
        elapsed := time.Since(start)

        if sendErr == nil && code >= 200 && code < 300 {
            _, _ = w.DB.Exec(ctx, `
                UPDATE delivery_attempt SET status='succeeded', succeeded_at=now(), http_code=$2, response_headers=$3::jsonb, response_body=$4, elapsed_ms=$5, worker_name=$6, worker_version=$7 WHERE attempt_id=$1
            `, attempt.AttemptID, code, toJSON(respHdrs), limitBody(respBody), elapsed.Milliseconds(), w.Name, w.Version)
            // health success
            _, _ = w.DB.Exec(ctx, `INSERT INTO destination_health(destination_id, success_count) VALUES($1,1)
                ON CONFLICT (destination_id) DO UPDATE SET success_count=destination_health.success_count+1`, dest.DestinationID)
        } else {
            var nextAt *time.Time
            // 429 Retry-After
            if code == 429 {
                if ra := parseRetryAfter(respHdrs["Retry-After"]); ra > 0 {
                    t := time.Now().Add(ra)
                    nextAt = &t
                }
            }
            if nextAt == nil {
                if d := nextRetry(int(attempt.AttemptNo)); d > 0 {
                    t := time.Now().Add(d)
                    nextAt = &t
                }
            }
            if nextAt != nil {
                _, _ = w.DB.Exec(ctx, `
                    UPDATE delivery_attempt SET status='pending', next_at=$2, attempt_no=attempt_no+1, http_code=$3, response_headers=$4::jsonb, response_body=$5, response_error=$6, elapsed_ms=$7 WHERE attempt_id=$1
                `, attempt.AttemptID, *nextAt, code, toJSON(respHdrs), limitBody(respBody), errString(sendErr), elapsed.Milliseconds())
            } else {
                _, _ = w.DB.Exec(ctx, `
                    UPDATE delivery_attempt SET status='failed', failed_at=now(), http_code=$2, response_headers=$3::jsonb, response_body=$4, response_error=$5, elapsed_ms=$6 WHERE attempt_id=$1
                `, attempt.AttemptID, code, toJSON(respHdrs), limitBody(respBody), errString(sendErr), elapsed.Milliseconds())
            }
            // health failure
            _, _ = w.DB.Exec(ctx, `INSERT INTO destination_health(destination_id, failure_count) VALUES($1,1)
                ON CONFLICT (destination_id) DO UPDATE SET failure_count=destination_health.failure_count+1`, dest.DestinationID)
            // open breaker if needed
            var success, failure int
            _ = w.DB.QueryRow(ctx, `SELECT success_count, failure_count FROM destination_health WHERE destination_id=$1`, dest.DestinationID).Scan(&success, &failure)
            total := success + failure
            if total >= int(dest.BreakerMinRequests) {
                if float64(failure)/float64(total) >= dest.BreakerFailureRatio {
                    until := time.Now().Add(time.Duration(dest.BreakerCooldownS) * time.Second)
                    _, _ = w.DB.Exec(ctx, `UPDATE destination_health SET open_until=$2 WHERE destination_id=$1`, dest.DestinationID, until)
                }
            }
        }

        limiter.Done(ctx, dest.DestinationID)
        _ = routeID
    }
}

type attemptRow struct {
    AttemptID string
    EventID   string
    RouteID   string
    AttemptNo int32
}

type destRow struct {
    DestinationID string
    URL           string
    Headers       map[string]string
    Secret        *string
    ConnectTimeoutS int32
    TimeoutS      int32
    VerifyTLS     bool
    MaxRPS        float64
    Burst         int32
    MaxInflight   int32
    BreakerFailureRatio float64
    BreakerMinRequests  int32
    BreakerCooldownS    int32
    AppendPath   bool
}

type evtRow struct {
    Body        []byte
    Headers     map[string]string
    ContentType string
    EventID     string
    SourceID    string
    Method      string
    Path        string
    Query       string
}

func (w *Worker) pickNext(ctx context.Context) (attemptRow, bool, error) {
    tx, err := w.DB.BeginTx(ctx, pgx.TxOptions{})
    if err != nil { return attemptRow{}, false, err }
    defer func() { _ = tx.Rollback(ctx) }()
    var at attemptRow
    err = tx.QueryRow(ctx, `
        WITH next AS (
          SELECT attempt_id, event_id, route_id, attempt_no
          FROM delivery_attempt
          WHERE status='pending' AND next_at <= now()
          ORDER BY next_at ASC
          LIMIT 1
          FOR UPDATE SKIP LOCKED
        )
        UPDATE delivery_attempt da
        SET status='picked', picked_at=now(), worker_name=$1, worker_version=$2
        FROM next
        WHERE da.attempt_id = next.attempt_id
        RETURNING da.attempt_id, da.event_id, da.route_id, da.attempt_no
    `, w.Name, w.Version).Scan(&at.AttemptID, &at.EventID, &at.RouteID, &at.AttemptNo)
    if errors.Is(err, pgx.ErrNoRows) {
        return attemptRow{}, false, tx.Commit(ctx)
    }
    if err != nil {
        return attemptRow{}, false, err
    }
    if err := tx.Commit(ctx); err != nil { return attemptRow{}, false, err }
    return at, true, nil
}

func (w *Worker) loadAttemptContext(ctx context.Context, attemptID string) (destRow, string, evtRow, error) {
    var d destRow
    var rID string
    // route + dest
    var hdrJSON []byte
    err := w.DB.QueryRow(ctx, `
        SELECT d.destination_id, d.url, d.headers, d.secret, d.connect_timeout_s, d.timeout_s, d.verify_tls,
               d.max_rps, d.burst, d.max_inflight,
               d.breaker_failure_ratio, d.breaker_min_requests, d.breaker_cooldown_s,
               d.append_path,
               da.route_id
        FROM delivery_attempt da
        JOIN route r ON r.route_id = da.route_id
        JOIN destination d ON d.destination_id = r.destination_id
        WHERE da.attempt_id=$1
    `, attemptID).Scan(
        &d.DestinationID, &d.URL, &hdrJSON, &d.Secret, &d.ConnectTimeoutS, &d.TimeoutS, &d.VerifyTLS,
        &d.MaxRPS, &d.Burst, &d.MaxInflight, &d.BreakerFailureRatio, &d.BreakerMinRequests, &d.BreakerCooldownS,
        &d.AppendPath,
        &rID,
    )
    if err != nil { return d, "", evtRow{}, err }
    if len(hdrJSON) > 0 {
        _ = json.Unmarshal(hdrJSON, &d.Headers)
    }
    var e evtRow
    var evHdrJSON []byte
    err = w.DB.QueryRow(ctx, `
        SELECT e.body, e.headers, coalesce(e.content_type,''), e.event_id::text, e.source_id::text,
               coalesce(e.method,''), coalesce(e.path,''), coalesce(e.query,'')
        FROM delivery_attempt da JOIN event e ON e.event_id = da.event_id
        WHERE da.attempt_id=$1
    `, attemptID).Scan(&e.Body, &evHdrJSON, &e.ContentType, &e.EventID, &e.SourceID, &e.Method, &e.Path, &e.Query)
    if err != nil { return d, "", evtRow{}, err }
    if len(evHdrJSON) > 0 {
        _ = json.Unmarshal(evHdrJSON, &e.Headers)
    }
    return d, rID, e, nil
}

func (w *Worker) deliver(ctx context.Context, d destRow, e evtRow) (int, map[string]string, []byte, error) {
    u, err := url.Parse(d.URL)
    if err != nil { return 0, nil, nil, err }
    // Optionally append inbound path+query to destination URL
    if d.AppendPath {
        // Path join preserving slashes
        basePath := strings.TrimSuffix(u.Path, "/")
        inPath := e.Path
        if inPath == "" { inPath = "/" }
        if !strings.HasPrefix(inPath, "/") { inPath = "/" + inPath }
        u.Path = basePath + inPath
        if e.Query != "" {
            if u.RawQuery != "" { u.RawQuery = u.RawQuery + "&" + e.Query } else { u.RawQuery = e.Query }
        }
    }
    ip, host, err := ssrf.ResolveAndPin(u)
    if err != nil { return 0, nil, nil, err }
    addr := net.JoinHostPort(ip.String(), u.Port())
    if u.Port() == "" {
        if u.Scheme == "https" { addr = net.JoinHostPort(ip.String(), "443") } else { addr = net.JoinHostPort(ip.String(), "80") }
    }

    dialer := &net.Dialer{Timeout: time.Duration(d.ConnectTimeoutS) * time.Second}
    transport := &http.Transport{
        DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
            return dialer.DialContext(ctx, network, addr)
        },
        TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: !d.VerifyTLS},
    }
    client := &http.Client{Transport: transport, Timeout: time.Duration(d.TimeoutS) * time.Second}
    client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

    method := e.Method
    if method == "" { method = http.MethodPost }
    req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(e.Body))
    if err != nil { return 0, nil, nil, err }
    if e.ContentType != "" { req.Header.Set("Content-Type", e.ContentType) }
    // static headers
    for k, v := range d.Headers { req.Header.Set(k, v) }
    // dynamic headers
    req.Header.Set("X-Source-Id", e.SourceID)
    req.Header.Set("X-Event-Id", e.EventID)
    if d.Secret != nil && *d.Secret != "" {
        ts := fmt.Sprintf("%d", time.Now().Unix())
        mac := hmac.New(sha256.New, []byte(*d.Secret))
        mac.Write([]byte(ts))
        mac.Write([]byte("\n"))
        mac.Write(e.Body)
        sig := hex.EncodeToString(mac.Sum(nil))
        req.Header.Set("X-Webhook-Signature", fmt.Sprintf("t=%s,v1=%s", ts, sig))
    }

    resp, err := client.Do(req)
    if err != nil { return 0, nil, nil, err }
    defer resp.Body.Close()
    hdrs := map[string]string{}
    for k, v := range resp.Header { if len(v) > 0 { hdrs[k] = v[0] } }
    rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    return resp.StatusCode, hdrs, rb, nil
}

func parseRetryAfter(v string) time.Duration {
    if v == "" { return 0 }
    if secs, err := time.ParseDuration(v + "s"); err == nil { return secs }
    if t, err := http.ParseTime(v); err == nil {
        d := time.Until(t)
        if d < 0 { return 0 }
        return d
    }
    return 0
}

func nextRetry(attemptNo int) time.Duration {
    const maxFast = 30
    const minFast = 5 * time.Second
    const maxFastDelay = 5 * time.Minute
    const slowDelay = time.Hour
    if attemptNo < maxFast {
        d := time.Duration(attemptNo) * minFast
        if d > maxFastDelay { d = maxFastDelay }
        if d <= 0 { d = minFast }
        return d
    }
    if attemptNo < maxFast+30 {
        return slowDelay
    }
    return 0
}

func toJSON(m map[string]string) string {
    b, _ := json.Marshal(m)
    return string(b)
}

func limitBody(b []byte) string {
    const max = 1 << 16
    if len(b) > max {
        return string(b[:max])
    }
    return string(b)
}

func errString(err error) *string {
    if err == nil { return nil }
    s := err.Error()
    return &s
}

// housekeeping periodically deletes old events that have no in-flight work
func (w *Worker) housekeeping(ctx context.Context) {
    days := 7
    if v := os.Getenv("RETENTION_DAYS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 { days = n }
    }
    ticker := time.NewTicker(time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
            cmd, err := w.DB.Exec(ctx, `
                DELETE FROM event e
                WHERE e.received_at < $1
                  AND NOT EXISTS (
                    SELECT 1 FROM delivery_attempt da
                    WHERE da.event_id = e.event_id
                      AND da.status IN ('pending','picked')
                  )
            `, cutoff)
            if err != nil {
                w.Logger("housekeeping_error", "error", err)
            } else if cmd.RowsAffected() > 0 {
                w.Logger("housekeeping_deleted", "rows", cmd.RowsAffected())
            }
        }
    }
}

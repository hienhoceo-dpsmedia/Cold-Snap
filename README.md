# Cold Snap — Smart webhook throttling that keeps your n8n cool under pressure

Cold Snap is a minimal, self-hosted webhook gateway that fronts n8n (or any app) to enforce webhook rate limits, retries, and safety — keeping your workflows cool under pressure.

What’s implemented now (MVP):
- Postgres schema + auto-migrations on start
- Ingest API: `POST /ingest/{token}` or `POST /ingest` with `Authorization: Bearer <token>`
- Event persistence: raw body, headers, content-type, source IP, optional Idempotency-Key
- Request fidelity: stores `method`, relative `path` (after `/ingest/{token}`), and `query`
- Attempt fan-out: one `delivery_attempt` per enabled route (simple content-type LIKE filter)
- Worker: PG `FOR UPDATE SKIP LOCKED` loop, SSRF guard with IP pinning, HTTP delivery with timeouts
- Redis: token-bucket rate limiter + max inflight per destination
- Basic circuit breaker counters and open-until deferral (cooldown)
- Optional path passthrough: destination can `append_path` to forward inbound path/query
- Replay: `POST /events/{event_id}/replay` creates fresh attempts
- Retention: worker cleans up events older than `RETENTION_DAYS` (default 7) with no in-flight attempts

Not yet: Admin UI/CRUD, metrics, traces. Easy to add next.

## Run with Docker Compose

```
# Build and start (Docker)
docker compose up --build

# API available at http://localhost:8080/healthz
```

Postgres is exposed on `localhost:5432` (user `hook`, password `hook`, db `hook`).

## Seeding minimal data

Connect to Postgres and create a source, destination, and route:

```
psql postgres://hook:hook@localhost:5432/hook <<'SQL'
INSERT INTO source (name, token) VALUES ('demo-source', 'demo_token')
  ON CONFLICT (name) DO UPDATE SET token=EXCLUDED.token;

INSERT INTO destination (name, url, headers, secret, max_rps, burst, max_inflight)
VALUES ('echo-dest', 'https://postman-echo.com/post', '{"X-Static": "1"}', NULL, 5.0, 5, 3)
  ON CONFLICT (name) DO UPDATE SET url=EXCLUDED.url;

INSERT INTO route (source_id, destination_id, enabled, content_type_like)
SELECT s.source_id, d.destination_id, true, 'application/json%'
FROM source s, destination d
WHERE s.name='demo-source' AND d.name='echo-dest'
ON CONFLICT (source_id, destination_id) DO NOTHING;
SQL
```

## Send a test webhook

```
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}' \
  http://localhost:8080/ingest/demo_token
```

You should see `202 Accepted` and attempts created. The worker will deliver to the destination with rate limiting and SSRF guard.

## n8n Quickstart

Run gateway + n8n locally (adds an n8n container) and seed a route that forwards to n8n with path passthrough.

```
npm run up:n8n
npm run seed:n8n

# Then send a webhook to the gateway; it forwards to n8n:
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -d '{"hello":"n8n"}' \
  http://localhost:8080/ingest/n8n_token/webhook/test

# The path after /ingest/n8n_token (here /webhook/test) is appended to the n8n base URL
```

Open n8n at http://localhost:5678 and configure a Test Webhook (e.g. `/webhook/test`). The gateway will protect n8n with rate limiting and retries.

Note: This project is not affiliated with n8n GmbH. “n8n” is a trademark of its respective owner.

## Deploy with Portainer (Stack)

Portainer makes deploying the full stack easy using the Git repo:

- In Portainer, go to `Stacks` → `Add stack` → `Git repository`.
- Repository URL: your GitHub repo URL for this project.
- Compose path: `docker-compose.yml` (leave build context as repo root).
- Environment variables: defaults are embedded; adjust if needed.
- Deploy the stack. Portainer builds the images from the Dockerfile and starts `api`, `worker`, `postgres`, `redis`.

After deploy:
- Open `http://<your-host>:8080/healthz` for health.
- Seed sample data in Portainer: `Stacks` → select stack → `Containers` → open `postgres` → `Console` → run `psql -U hook -d hook` and paste the contents of `seeds/demo.sql`.
  - For n8n demo, paste `seeds/n8n-demo.sql` and ensure an `n8n` container is reachable at `http://n8n:5678` (use `docker-compose.n8n.yml` or your own n8n deployment).

## Install/run using npm scripts

If you prefer npm to wrap Docker Compose:

```
npm run up          # build and start all services
npm run logs        # follow logs
npm run seed        # seed demo source/destination/route
npm run down        # stop and remove containers + volumes
```

Requirements: Docker Engine and Node.js (>=18). The npm scripts just call `docker compose` under the hood.

## Configuration

- `ROLE`: `api` or `worker` (Docker Compose runs both)
- `API_PORT`: default 8080
- `DATABASE_URL`: Postgres connection string
- `REDIS_URL`: Redis connection string
- `WORKER_NAME`, `WORKER_VERSION`: optional labels
- `RETENTION_DAYS`: default 7 (worker)

## Next steps

- Admin REST + UI (React Admin)
- Retry metrics and Prometheus export
- Rich routing filters and replay endpoints
- Health window sliding/housekeeping
 - Destination ordering and fallback (first/all)

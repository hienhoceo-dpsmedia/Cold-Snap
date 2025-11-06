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

## Quick Install (Portainer + Nginx Proxy Manager)

Two steps: 1) deploy the stack in Portainer, 2) publish your domain with Nginx Proxy Manager (NPM).

Step 1 — Portainer stack (from Git)
- Stacks → Add stack → Repository
- Repository URL: `https://github.com/<you>/Cold-Snap`
- Reference: `refs/heads/main`
- Compose path: `docker-compose.yml`
- Environment variables: set `ADMIN_TOKEN` to a strong random value
- Deploy (services: api on 8080, worker, postgres, redis)

Step 2 — Nginx Proxy Manager (proxy host)

Option A — shared Docker network (recommended)
1. Portainer → Networks → Add network → name: `npm_proxy`
2. Add an extra compose file when deploying: `docker-compose.npm.yml`
3. In NPM: Hosts → Proxy Hosts → Add
   - Domain Names: `coldsnap.yourdomain.com`
   - Scheme: `http`
   - Forward Hostname/IP: `api`
   - Forward Port: `8080`
   - Advanced → Custom Nginx: `client_max_body_size 10m;`
   - SSL: Request a new certificate → Force SSL (optional)

Option B — no shared network
- Forward Hostname/IP: your server’s LAN IP (or docker bridge `172.17.0.1`)
- Forward Port: `8080`
- Same SSL and Advanced settings as above

Verify: open `https://coldsnap.yourdomain.com/healthz`

Seed (optional): in Portainer → Containers → postgres → Console → `psql -U hook -d hook`, paste `seeds/demo.sql`. For n8n demo use `seeds/n8n-demo.sql`.

## Admin REST (create sources/destinations/routes)

Cold Snap can run independently of n8n. You can create multiple sources (tokens) and map them to destinations with per‑destination controls. Protect admin endpoints with `ADMIN_TOKEN` env and call with `Authorization: Bearer <token>`.

- Create source
```
curl -s -X POST http://localhost:8080/admin/sources \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"shopify","max_body_bytes":1048576}'
```
- List sources: `curl -s -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/sources`

- Create destination
```
curl -s -X POST http://localhost:8080/admin/destinations \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"internal-api",
    "url":"https://example.com/hooks",
    "headers": {"X-App":"ColdSnap"},
    "max_rps": 2.0,
    "burst": 5,
    "max_inflight": 2,
    "append_path": true
  }'
```
- List destinations: `curl -s -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/destinations`

- Create route (map source → destination)
```
curl -s -X POST http://localhost:8080/admin/routes \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"source_name":"shopify","destination_name":"internal-api","content_type_like":"application/json%"}'
```
- List routes: `curl -s -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/admin/routes`

Now send webhooks to `POST /ingest/{token}` for any created source; the worker delivers to mapped destinations with rate limits, retries, and SSRF guard.

## Install/run using npm scripts

If you prefer npm to wrap Docker Compose:

```
npm run up          # build and start all services
npm run logs        # follow logs
npm run seed        # seed demo source/destination/route
npm run down        # stop and remove containers + volumes
```

Requirements: Docker Engine and Node.js (>=18). The npm scripts just call `docker compose` under the hood.

## Install on Your Domain (Nginx)

This example shows how to serve Cold Snap at `http://coldsnap.dpsmedia.vn/` (and optionally HTTPS) while running services via npm/Compose.

1) DNS
- Create an A/AAAA record for `coldsnap.dpsmedia.vn` pointing to your server.

2) Server prerequisites
- Ubuntu/Debian example:
  - Install Docker and Compose plugin (per Docker docs)
  - Install Node.js 18+ (e.g., via nvm)
  - Install Nginx: `sudo apt-get install nginx`

3) Clone and start Cold Snap
```
git clone https://github.com/<you>/Cold-Snap.git
cd Cold-Snap

# Optional: create a .env (Compose will read it) to set secrets/ports
cat > .env <<EOF
ADMIN_TOKEN=$(openssl rand -hex 16)
API_PORT=8080
RETENTION_DAYS=7
EOF

npm run up
```

4) Configure Nginx reverse proxy
- Copy `deploy/nginx.coldsnap.example.conf` to `/etc/nginx/sites-available/coldsnap.conf` and adjust `server_name` and `client_max_body_size`.
- Symlink and reload:
```
sudo ln -s /etc/nginx/sites-available/coldsnap.conf /etc/nginx/sites-enabled/coldsnap.conf
sudo nginx -t && sudo systemctl reload nginx
```
- You should now reach the API at `http://coldsnap.dpsmedia.vn/healthz`.

5) Optional HTTPS with Let’s Encrypt (certbot)
```
sudo apt-get install certbot python3-certbot-nginx
sudo certbot --nginx -d coldsnap.dpsmedia.vn
```
Certbot updates the Nginx conf to redirect HTTP→HTTPS and install certs.

6) Create sources/destinations via Admin REST
- Set your admin token: `export ADMIN_TOKEN=$(grep ADMIN_TOKEN .env | cut -d= -f2)`
- Follow the Admin REST examples below to create sources, destinations, and routes.

Notes
- Keep Nginx `client_max_body_size` ≥ the per‑source `max_body_bytes` (default 1 MiB).
- If you expose only HTTPS externally, keep Compose ports internal and proxy to `127.0.0.1:8080`.
- Health: `/healthz` on your domain; Ingest: `/ingest/{token}`.

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

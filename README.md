<div align="center">

# 🧊 Cold Snap

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white)](https://www.docker.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-336791?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-DC382D?style=flat-square&logo=redis&logoColor=white)](https://redis.io/)
[![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

**Smart webhook throttling that keeps your n8n cool under pressure** ❄️

A minimal, self-hosted webhook gateway that fronts n8n (or any app) to enforce webhook rate limits, retries, and safety — keeping your workflows running smoothly under pressure.

[🚀 Quick Start](#-quick-start) • [📖 Documentation](#-documentation) • [🎯 Features](#-features) • [🔧 Configuration](#-configuration)

</div>

## 🎯 About Cold Snap

### 🔒 Branding & Independence
- **n8n Focused**: The branding references n8n because many users need to protect n8n webhooks
- **Zero Integration Required**: Cold Snap does not integrate with or require access to n8n internals
- **Smart Proxy**: Acts as an intelligent reverse proxy with buffering, backpressure, and retries
- **Secure**: You never share n8n credentials/keys/tokens with Cold Snap
- **Universal**: Works with any webhook consumer (n8n, Zapier, custom apps, internal APIs)

---

## ✨ Features

### 🚀 **Core MVP Features**
- 🗄️ **PostgreSQL** schema with auto-migrations on startup
- 📥 **Ingest API**: `POST /ingest/{token}` or `POST /ingest` with `Authorization: Bearer <token>`
- 💾 **Event Persistence**: Raw body, headers, content-type, source IP, optional Idempotency-Key
- 🔍 **Request Fidelity**: Stores `method`, relative `path`, and `query` parameters
- 🎯 **Smart Routing**: One delivery attempt per enabled route with content-type filtering
- ⚡ **High-Performance Worker**: PostgreSQL `FOR UPDATE SKIP LOCKED` loop with SSRF protection
- 🚦 **Rate Limiting**: Redis-based token-bucket limiter + max inflight control per destination
- 🛡️ **Circuit Breaker**: Automatic protection with cooldown and retry logic
- 🔄 **Path Passthrough**: Destination can `append_path` to forward inbound path/query
- 🔁 **Event Replay**: `POST /events/{event_id}/replay` creates fresh delivery attempts
- 🧹 **Auto Cleanup**: Worker removes events older than `RETENTION_DAYS` (default 7)

### 🎨 **Admin Interface**
- 🌐 **Web UI**: Modern admin console for monitoring and management
- 📊 **Real-time Dashboard**: Live webhook status and metrics
- 🔧 **Management**: Create/edit sources, destinations, and routes via UI
- 📱 **Responsive**: Works on desktop and mobile devices

### 🛠️ **Coming Soon**
- 📈 **Advanced Metrics**: Prometheus integration and detailed analytics
- 🔍 **Request Tracing**: End-to-end webhook flow visualization
- 🎛️ **Enhanced Routing**: Advanced filters and conditional logic
- 🔄 **Retry Strategies**: Configurable backoff and retry policies

## 🚀 Quick Start

### 🐳 Docker Compose (Recommended)

```bash
# Clone and build
git clone https://github.com/hienhoceo-dpsmedia/Cold-Snap.git
cd Cold-Snap

# Build and start all services
docker compose up --build

# API available at http://localhost:8080/healthz
# Admin UI available at http://localhost:8080/console/
```

**📊 Services Started:**
- 🌐 **API Server**: `http://localhost:8080`
- 🗄️ **PostgreSQL**: `localhost:5432` (user: `hook`, password: `hook`, db: `hook`)
- 🚦 **Redis**: `localhost:6379`
- ⚙️ **Worker**: Background delivery processor

---

## 🌱 Quick Setup

### Option 1: **Use the Admin UI** 🎨

1. Open `http://localhost:8080/console/` in your browser
2. Enter your admin token (or use the default)
3. Create sources, destinations, and routes via the web interface

### Option 2: **Database Setup** 💾

Connect to PostgreSQL and create test data:

```sql
-- Connect to database
psql postgres://hook:hook@localhost:5432/hook

-- Create a source with authentication token
INSERT INTO source (name, token) VALUES ('demo-source', 'demo_token')
  ON CONFLICT (name) DO UPDATE SET token=EXCLUDED.token;

-- Create a destination (where webhooks get delivered)
INSERT INTO destination (name, url, headers, secret, max_rps, burst, max_inflight)
VALUES ('echo-dest', 'https://postman-echo.com/post', '{"X-Static": "1"}', NULL, 5.0, 5, 3)
  ON CONFLICT (name) DO UPDATE SET url=EXCLUDED.url;

-- Create a route (maps source → destination)
INSERT INTO route (source_id, destination_id, enabled, content_type_like)
SELECT s.source_id, d.destination_id, true, 'application/json%'
FROM source s, destination d
WHERE s.name='demo-source' AND d.name='echo-dest'
ON CONFLICT (source_id, destination_id) DO NOTHING;
```

---

## 🧪 Test Your Setup

### Send a Test Webhook

```bash
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' \
  http://localhost:8080/ingest/demo_token
```

**✅ Expected Response:** `202 Accepted`
The worker will deliver the webhook to your destination with rate limiting and SSRF protection.

---

## 🔥 n8n Integration (Zero Setup Required!)

### Quick Start with n8n

```bash
# Start gateway + n8n together
npm run up:n8n
npm run seed:n8n

# Send webhook to gateway (forwards to n8n)
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -d '{"hello":"n8n","workflow":"test"}' \
  http://localhost:8080/ingest/n8n_token/webhook/test
```

🎯 **Open n8n**: http://localhost:5678
Configure a webhook trigger at `/webhook/test` - Cold Snap will protect it with rate limiting and retries.

### 🔧 n8n Setup Options

#### Option A: Full URL
- **Destination URL**: `https://n8n.example.com/webhook/<your-id>`
- **Append Path**: `false`
- **Result**: Direct forwarding to specific webhook

#### Option B: Base URL + Path Forwarding ⭐
- **Destination URL**: `https://n8n.example.com`
- **Append Path**: `true`
- **Result**: `POST /ingest/token/webhook/test` → `https://n8n.example.com/webhook/test`

> 💡 **Security First**: You never share n8n credentials with Cold Snap. It only forwards HTTP requests to your webhook URLs.

> ⚠️ **Disclaimer**: This project is not affiliated with n8n GmbH. "n8n" is a trademark of its respective owner.

---

## 🏗️ Production Deployment

### 📋 Portainer + Nginx Proxy Manager

**Step 1: Deploy Stack in Portainer**

1. **Stacks** → **Add stack** → **Repository**
2. **Repository URL**: `https://github.com/hienhoceo-dpsmedia/Cold-Snap`
3. **Reference**: `refs/heads/main`
4. **Compose path**: `docker-compose.yml`
5. **Environment Variables (Repository mode)**
   - In Portainer's Repository deploy, the key=value editor is not used. Portainer reads variables from a file named `stack.env` at the repo root.
   - Fork this repo, copy `stack.env.example` to `stack.env`, edit values (at least `ADMIN_TOKEN`), commit, then point Portainer to your fork.
   - Example `stack.env`:
     ```env
     ADMIN_TOKEN=change_me_to_a_strong_random
     API_PORT=8080
     RETENTION_DAYS=7
     DATABASE_URL=postgres://hook:hook@postgres:5432/hook?sslmode=disable
     REDIS_URL=redis://redis:6379/0
     ```
6. **Deploy** 🚀 (services: api on 8080, worker, postgres, redis)

**Step 2: Configure Nginx Proxy Manager**

#### Option A: Shared Docker Network ⭐ (Recommended)
```bash
# Create shared network
docker network create npm_proxy

# Use docker-compose.npm.yml for extra network configuration
```

In **NPM**: **Hosts** → **Proxy Hosts** → **Add**
- **Domain Names**: `coldsnap.yourdomain.com`
- **Scheme**: `http`
- **Forward Hostname/IP**: `api`
- **Forward Port**: `8080`
- **Advanced** → **Custom Nginx**: `client_max_body_size 10m;`
- **SSL**: Request certificate → Force SSL (recommended)

#### Option B: Direct IP
- **Forward Hostname/IP**: Your server's LAN IP (e.g., `172.17.0.1`)
- **Forward Port**: `8080`
- Same SSL and Advanced settings as above

**✅ Verify**: Open `https://coldsnap.yourdomain.com/healthz`

> 💡 **Pro Tip**: Use the web UI at `/console/` to manage sources, destinations, and routes - no SQL required!

---

## 🛠️ API Documentation

### 🔐 Admin REST API

Protect admin endpoints with `ADMIN_TOKEN` environment variable and authenticate using `Authorization: Bearer <token>`.

#### 📥 Create Source
```bash
curl -s -X POST http://localhost:8080/admin/sources \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "shopify",
    "max_body_bytes": 1048576
  }'
```

#### 📤 Create Destination
```bash
curl -s -X POST http://localhost:8080/admin/destinations \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "internal-api",
    "url": "https://example.com/hooks",
    "headers": {"X-App": "ColdSnap"},
    "max_rps": 2.0,
    "burst": 5,
    "max_inflight": 2,
    "append_path": true
  }'
```

#### 🔗 Create Route (Connect Source → Destination)
```bash
curl -s -X POST http://localhost:8080/admin/routes \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "source_name": "shopify",
    "destination_name": "internal-api",
    "content_type_like": "application/json%"
  }'
```

#### 📋 List Resources
```bash
# List all sources
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/admin/sources

# List all destinations
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/admin/destinations

# List all routes
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/admin/routes
```

### 📡 Ingest API
Send webhooks to `POST /ingest/{token}` for any created source. The worker delivers to mapped destinations with:

- ✅ **Rate limiting** and **retry logic**
- ✅ **SSRF protection** and **security guards**
- ✅ **Event persistence** and **replay capabilities**

---

## 🚀 Development Setup

### 📦 Using NPM Scripts

If you prefer npm to wrap Docker Compose:

```bash
npm run up          # 🚀 build and start all services
npm run logs        # 📋 follow logs
npm run seed        # 🌱 seed demo source/destination/route
npm run down        # 🛑 stop and remove containers + volumes
npm run up:n8n      # 🔥 start with n8n integration
npm run seed:n8n    # 🌱 seed n8n demo data
```

**Requirements:**
- 🐳 Docker Engine
- 🟢 Node.js (>=18)
- The npm scripts are convenient wrappers around `docker compose`

---

## 🌍 Custom Domain Setup

### 🔧 Manual Nginx Deployment

Deploy Cold Snap on your own domain with this step-by-step guide:

#### 1️⃣ DNS Configuration
```bash
# Create A/AAAA record pointing to your server
coldsnap.yourdomain.com → YOUR_SERVER_IP
```

#### 2️⃣ Server Prerequisites
```bash
# Ubuntu/Debian example
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo apt-get install docker-compose-plugin

# Install Node.js 18+
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash
nvm install 18

# Install Nginx
sudo apt-get update && sudo apt-get install nginx
```

#### 3️⃣ Deploy Cold Snap
```bash
# Clone and setup
git clone https://github.com/hienhoceo-dpsmedia/Cold-Snap.git
cd Cold-Snap

# Create environment file
cat > .env <<EOF
ADMIN_TOKEN=$(openssl rand -hex 16)
API_PORT=8080
RETENTION_DAYS=7
EOF

# Start services
npm run up
```

#### 4️⃣ Configure Nginx Reverse Proxy
```bash
# Copy and customize Nginx config
sudo cp deploy/nginx.coldsnap.example.conf /etc/nginx/sites-available/coldsnap.conf

# Edit server_name and settings
sudo nano /etc/nginx/sites-available/coldsnap.conf

# Enable site
sudo ln -s /etc/nginx/sites-available/coldsnap.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

# Test: http://coldsnap.yourdomain.com/healthz ✅
```

#### 5️⃣ Enable HTTPS with Let's Encrypt
```bash
sudo apt-get install certbot python3-certbot-nginx
sudo certbot --nginx -d coldsnap.yourdomain.com
```

#### 6️⃣ Configure via Admin API
```bash
export ADMIN_TOKEN=$(grep ADMIN_TOKEN .env | cut -d= -f2)
# Follow API examples above to create sources, destinations, routes
```

### 💡 Production Tips
- **Body Size**: Keep Nginx `client_max_body_size` ≥ source `max_body_bytes` (default 1MB)
- **Security**: For HTTPS-only, proxy to `127.0.0.1:8080` instead of exposing ports
- **Monitoring**: Use `/healthz` for health checks and `/console/` for management

---

## ⚙️ Configuration

### Environment Variables
```bash
# Core Service Configuration
ROLE=api|worker                    # Service role (Docker Compose runs both)
API_PORT=8080                     # API server port
ADMIN_TOKEN=your_secret_token     # Admin authentication token

# Database Configuration
DATABASE_URL=postgres://...       # PostgreSQL connection string
REDIS_URL=redis://...             # Redis connection string

# Worker Configuration
WORKER_NAME=worker-1              # Optional worker identifier
WORKER_VERSION=v1.0.0             # Optional version label
RETENTION_DAYS=7                  # Event retention period (default: 7)

# Performance Tuning
MAX_WORKERS=10                    # Maximum concurrent workers
BATCH_SIZE=100                    # Event processing batch size
TIMEOUT_SECONDS=30                # Request timeout
```

### 🚀 Advanced Configuration
- **Rate Limiting**: Per-destination `max_rps`, `burst`, and `max_inflight`
- **Circuit Breaking**: Automatic failure detection and recovery
- **SSRF Protection**: IP whitelisting and network security
- **Retry Logic**: Configurable backoff strategies (coming soon)

---

## 🗺️ Roadmap

### ✅ **Completed (MVP)**
- [x] Core webhook ingestion and delivery
- [x] PostgreSQL event persistence
- [x] Redis rate limiting
- [x] Basic admin REST API
- [x] Web admin console
- [x] Docker deployment

### 🚧 **In Development**
- [ ] Advanced retry strategies
- [ ] Prometheus metrics
- [ ] Enhanced routing filters

### 🎯 **Coming Soon**
- [ ] **Advanced Analytics**: Request tracing and detailed metrics
- [ ] **Enhanced Security**: IP whitelisting and advanced SSRF protection
- [ ] **Smart Routing**: Conditional logic and advanced filters
- [ ] **High Availability**: Multi-node clustering and failover
- [ ] **Integration Hub**: Pre-built connectors for popular services

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

### Development Setup
```bash
git clone https://github.com/hienhoceo-dpsmedia/Cold-Snap.git
cd Cold-Snap
npm run up
# Make your changes...
npm run test  # Coming soon
```

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🧊 **Cold Snap** - Keep Your Webhooks Cool

Made with ❄️ for reliable webhook delivery

<div align="center">

[🚀 Get Started](#-quick-start) • [📖 Documentation](#-documentation) • [🔧 Configuration](#-configuration) • [🤝 Contributing](#-contributing)

</div>

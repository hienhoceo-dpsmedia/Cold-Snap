# Webhook Proxy - Portainer Ready

A powerful webhook management tool that's ready to deploy with Portainer. This repository contains a fully configured webhook proxy system with Docker Compose setup for production deployment.

![Webhook Proxy Screenshot](screenshot.png)

## 🚀 Features

- **Multi-endpoint Management**: Add and manage multiple webhook endpoints
- **Message Persistence**: Save received messages for 7 days
- **Smart Forwarding**: Automatically forward incoming messages to one or more destinations
- **Flexible Routing**: Choose forwarding strategy (first in list, all in list)
- **Fallback Support**: Automatic fallback forwarding if primary destination is down
- **Message Replay**: Resend webhook data to destinations on demand
- **Modern UI**: Built with Next.js, Tailwind CSS, and tRPC
- **Production Ready**: Complete Docker setup with PostgreSQL and Redis

## 🐳 Quick Start with Portainer

### 1. Clone and Setup

```bash
git clone https://github.com/hienhoceo-dpsmedia/Cold-Snap.git
cd Cold-Snap
```

### 2. Configure Environment

Copy the environment template:

```bash
cp .env.production .env
```

Edit `.env` with your configuration:

```bash
# Required: GitHub OAuth (create app at https://github.com/settings/applications/new)
GITHUB_CLIENT_ID=your_github_client_id
GITHUB_CLIENT_SECRET=your_github_client_secret

# Required: Database password
POSTGRES_PASSWORD=your_secure_password

# Your domain
NEXT_PUBLIC_PRIMARY_DOMAIN=your-domain.com
```

### 3. Deploy with Portainer

1. Open Portainer web interface
2. Go to **Stacks** → **Add stack**
3. Set name: `webhook-proxy`
4. Copy contents of `docker-compose.portainer.yml`
5. Click **Deploy the stack**

That's it! Your webhook proxy will be running in minutes.

## 📋 Prerequisites

- Docker and Docker Compose
- Portainer (recommended) or Docker CLI
- Domain name (for production)
- GitHub OAuth App (for authentication)

## 🔧 Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_CLIENT_ID` | ✅ | GitHub OAuth Client ID |
| `GITHUB_CLIENT_SECRET` | ✅ | GitHub OAuth Client Secret |
| `POSTGRES_PASSWORD` | ✅ | Database password |
| `NEXT_PUBLIC_PRIMARY_DOMAIN` | ✅ | Your domain (e.g., webhook.yourdomain.com) |
| `ACME_EMAIL` | ❌ | Email for SSL certificates |

### GitHub OAuth Setup (Required for Login)

**Step 1: Create GitHub OAuth App**

1. Go to [GitHub Settings → Developer settings → OAuth Apps](https://github.com/settings/applications/new)
2. Click **"New OAuth App"**
3. Fill in the form:
   - **Application name**: `Webhook Proxy` (or any name you prefer)
   - **Homepage URL**: `https://your-domain.com` (replace with your actual domain)
   - **Authorization callback URL**: `https://your-domain.com/api/login/callback` (IMPORTANT: This exact URL is required)
   - **Application description**: Optional (e.g., "Webhook management system")
4. Click **"Register application"**

**Step 2: Get Credentials**

After creating the app, you'll see:
- **Client ID**: Copy this value
- **Client Secret**: Click "Generate a new client secret" and copy it

**Step 3: Update Environment File**

Edit your `.env` file:

```bash
# Replace with your actual values
GITHUB_CLIENT_ID=your_actual_github_client_id_here
GITHUB_CLIENT_SECRET=your_actual_github_client_secret_here

# Your domain where you'll deploy
NEXT_PUBLIC_PRIMARY_DOMAIN=your-domain.com
```

**Important Notes:**
- The callback URL **must** be exactly `https://your-domain.com/api/login/callback`
- For local development, use: `http://localhost:3000/api/login/callback`
- The domain in `NEXT_PUBLIC_PRIMARY_DOMAIN` must match the Homepage URL in GitHub OAuth app

## 🏗️ Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Webhook UI    │    │   PostgreSQL     │    │     Redis       │
│   (Next.js)     │    │   (Database)     │    │    (Cache)      │
│     Port 3000   │    │    Port 5432     │    │    Port 6379    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌──────────────────┐
                    │   Traefik        │
                    │ (Load Balancer)  │
                    │  Ports 80/443    │
                    └──────────────────┘
```

## 📁 Project Structure

```
├── src/                    # Next.js application source
│   ├── app/               # App router pages
│   ├── components/        # React components
│   └── server/           # Backend API and database
├── docker/               # Docker configuration files
├── docker-compose.portainer.yml  # Full Portainer stack
├── PORTAINER-STACK.yml   # Simplified stack
├── Dockerfile           # Multi-stage production build
├── DEPLOYMENT.md        # Detailed deployment guide
└── .env.production      # Environment template
```

## 🔌 Access Points

After deployment:

- **Webhook Proxy**: `https://your-domain.com`
- **Traefik Dashboard**: `https://traefik.your-domain.com`
- **Database Admin**: `https://pgadmin.your-domain.com`

## 📊 What Webhook Proxy Can Do

### 1. **Receive Webhooks**
- Create custom endpoints
- Support multiple webhooks simultaneously
- Automatic message validation and logging

### 2. **Forward Messages**
- Forward to multiple destinations
- Choose routing strategy:
  - **First Success**: Send to first destination, try others on failure
  - **Send to All**: Broadcast to all destinations
  - **Load Balance**: Distribute across destinations

### 3. **Monitor & Replay**
- View all received messages
- Check delivery status and logs
- Replay failed or successful deliveries
- 7-day message retention

### 4. **Manage Destinations**
- Add HTTP endpoints as destinations
- Configure custom headers and authentication
- Test destination connectivity

## 🛠️ Development

For local development:

```bash
# Install dependencies
npm install

# Copy environment
cp .env.example .env

# Start database
docker-compose up -d db

# Run database migrations
npm run db:push

# Start development server
npm run dev
```

Open http://localhost:3000

## 🔍 Monitoring

### Health Checks
- Application: `GET /api/health`
- Database: Automatic PostgreSQL health check
- Redis: Automatic Redis health check

### Logs
```bash
# Application logs
docker-compose logs -f webhook-proxy

# Database logs
docker-compose logs -f webhook-db
```

## 🔒 Security

- 🔐 GitHub OAuth authentication
- 🛡️ HTTPS with automatic SSL (Let's Encrypt)
- 🔒 Isolated Docker networks
- 🚫 No exposed database ports to internet
- 📝 Environment-based configuration

## 📈 Scaling

For high-traffic deployments:

- **Horizontal Scaling**: Run multiple app instances
- **Database Optimization**: Use connection pooling
- **Redis Clustering**: For high-throughput caching
- **Load Balancing**: Traefik automatically handles multiple instances

## 🆘 Troubleshooting

### Common Issues

**"GitHub OAuth not working"**
- Check callback URL matches GitHub app settings
- Verify Client ID and Secret are correct
- Ensure domain matches OAuth application

**"Database connection failed"**
- Check PostgreSQL container is running
- Verify DATABASE_URL format
- Check for port conflicts

**"Application won't start"**
- Review environment variables
- Check Docker logs: `docker-compose logs webhook-proxy`
- Verify all required services are healthy

### Reset Everything
```bash
docker-compose -f docker-compose.portainer.yml down -v
docker-compose -f docker-compose.portainer.yml up -d
```

## 📚 Documentation

- [Detailed Deployment Guide](DEPLOYMENT.md)
- [Next.js Documentation](https://nextjs.org/docs)
- [Drizzle ORM](https://orm.drizzle.team/)
- [tRPC](https://trpc.io/)

## 🤝 Contributing

1. Fork the repository
2. Create feature branch
3. Make your changes
4. Test thoroughly
5. Submit pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.

## 🌟 Credits

Based on the original [Webhook Proxy](https://github.com/un/webhook-proxy) by the UnJS team.

---

**Ready to deploy in 5 minutes with Portainer! 🚀**
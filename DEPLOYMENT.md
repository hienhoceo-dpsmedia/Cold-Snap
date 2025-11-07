# Webhook Proxy - Portainer Deployment Guide

This guide will help you deploy the Webhook Proxy application using Portainer with Docker Compose.

## Overview

Webhook Proxy is a powerful tool for managing webhooks with the following capabilities:
- Add multiple endpoints
- Save received messages (for 7 days)
- Automatically forward incoming messages to one or more destinations
- Choose forwarding strategy (send to: first in list, all in list)
- Support fallback forwarding (if first is down, forward to next)
- Replay webhook delivery (resend the data to destinations)

## Prerequisites

- Docker and Docker Compose installed
- Portainer running (optional but recommended)
- A domain name (for production deployment)
- GitHub OAuth App (required for authentication)

## Quick Start with Portainer

### 1. Set Up Environment Variables

Copy the environment file and customize it:

```bash
cp .env.production .env
```

Edit `.env` with your specific configuration:

```bash
# Application Configuration
NODE_ENV=production
NEXT_PUBLIC_PRIMARY_DOMAIN=your-domain.com

# Database Configuration
POSTGRES_PASSWORD=your_secure_password_here

# GitHub OAuth (REQUIRED)
GITHUB_CLIENT_ID=your_github_client_id
GITHUB_CLIENT_SECRET=your_github_client_secret

# SSL Certificate
ACME_EMAIL=admin@your-domain.com
```

### 2. Create GitHub OAuth App (Required for Login)

**Step-by-Step Guide:**

1. **Navigate to GitHub OAuth Settings**
   - Go to [GitHub Settings → Developer settings → OAuth Apps](https://github.com/settings/applications/new)
   - Click **"New OAuth App"**

2. **Fill in the OAuth Application Form**
   - **Application name**: `Webhook Proxy` (or any name you prefer)
   - **Homepage URL**: `https://your-domain.com` (replace with your actual domain)
   - **Authorization callback URL**: `https://your-domain.com/api/login/callback` ⚠️ **This exact URL is required**
   - **Application description**: `Webhook management system` (optional)

3. **Register and Get Credentials**
   - Click **"Register application"**
   - You'll see a **Client ID** - copy this value
   - Click **"Generate a new client secret"** - copy this secret immediately (you won't see it again)

4. **Update Your .env File**
   ```bash
   # Replace these with your actual GitHub OAuth values
   GITHUB_CLIENT_ID=iv1lx0a1b2c3d4e5f6g7h    # Replace with your actual Client ID
   GITHUB_CLIENT_SECRET=your_actual_secret_here   # Replace with your actual Client Secret

   # Must match your domain from GitHub OAuth app
   NEXT_PUBLIC_PRIMARY_DOMAIN=your-domain.com
   ```

**⚠️ Critical Requirements:**
- The callback URL **must** be exactly: `https://your-domain.com/api/login/callback`
- For local testing: use `http://localhost:3000/api/login/callback`
- The domain in `NEXT_PUBLIC_PRIMARY_DOMAIN` must match the GitHub OAuth Homepage URL
- Both Client ID and Client Secret are required for login to work

### 3. Deploy with Portainer

#### Option A: Using Portainer Stack Editor

1. Open Portainer web interface
2. Go to "Stacks" → "Add stack"
3. Set a name (e.g., "webhook-proxy")
4. Select "Web editor"
5. Copy the contents of `docker-compose.portainer.yml`
6. Click "Deploy the stack"

#### Option B: Using Docker Compose CLI

```bash
docker-compose -f docker-compose.portainer.yml up -d
```

### 4. Database Setup

The application will automatically create the database schema on first run. No manual migrations needed.

## Configuration Options

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NODE_ENV` | production | Application environment |
| `NEXT_PUBLIC_PRIMARY_DOMAIN` | localhost:3000 | Main domain for the application |
| `POSTGRES_DB` | webhookproxy | PostgreSQL database name |
| `POSTGRES_USER` | webhook | PostgreSQL username |
| `POSTGRES_PASSWORD` | - | PostgreSQL password (required) |
| `GITHUB_CLIENT_ID` | - | GitHub OAuth Client ID (required) |
| `GITHUB_CLIENT_SECRET` | - | GitHub OAuth Client Secret (required) |
| `REDIS_PASSWORD` | - | Redis password (optional) |
| `ACME_EMAIL` | - | Email for SSL certificates |

### Port Configuration

| Service | Default Port | Description |
|---------|--------------|-------------|
| Webhook Proxy | 3000 | Main application |
| PostgreSQL | 5432 | Database |
| Redis | 6379 | Cache |
| Traefik | 80, 443, 8080 | Reverse proxy and dashboard |

## Access Points

After deployment, you can access:

- **Main Application**: `https://your-domain.com`
- **Traefik Dashboard**: `https://traefik.your-domain.com` (if configured)
- **Database Admin**: `https://pgadmin.your-domain.com` (if configured)

## Development Setup

For local development:

1. Copy `.env.example` to `.env`
2. Start the database: `docker-compose up -d db`
3. Install dependencies: `npm install`
4. Run migrations: `npm run db:push`
5. Start development server: `npm run dev`

## Monitoring and Maintenance

### Health Checks

The application includes built-in health checks:
- Application: `http://localhost:3000/api/health`
- Database: Automatic PostgreSQL health check
- Redis: Automatic Redis health check

### Logs

View logs for each service:

```bash
# Application logs
docker-compose logs -f webhook-proxy

# Database logs
docker-compose logs -f webhook-db

# All services
docker-compose logs -f
```

### Backups

#### Database Backup

```bash
# Create backup
docker-compose exec webhook-db pg_dump -U webhook webhookproxy > backup.sql

# Restore backup
docker-compose exec -T webhook-db psql -U webhook webhookproxy < backup.sql
```

#### Volume Backup

```bash
# Backup all volumes
docker run --rm -v webhook_proxy_postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/postgres_backup.tar.gz -C /data .
```

## Security Considerations

1. **Change Default Passwords**: Always change the default passwords in the environment file
2. **Use HTTPS**: Ensure SSL certificates are properly configured
3. **Network Security**: Consider using internal networks for database access
4. **Regular Updates**: Keep Docker images and dependencies updated

## Troubleshooting

### Common Issues

1. **Application Won't Start**
   - Check environment variables are set correctly
   - Verify GitHub OAuth credentials
   - Check database connection

2. **Database Connection Errors**
   - Ensure PostgreSQL container is healthy
   - Verify connection string in DATABASE_URL
   - Check for port conflicts

3. **GitHub OAuth Issues**
   - Verify callback URL matches GitHub OAuth settings
   - Check Client ID and Secret are correct
   - Ensure domain matches OAuth application settings

### Reset Application

To completely reset the application:

```bash
# Stop all services
docker-compose -f docker-compose.portainer.yml down

# Remove volumes (WARNING: This deletes all data)
docker volume rm webhook_proxy_postgres_data webhook_proxy_redis_data

# Restart
docker-compose -f docker-compose.portainer.yml up -d
```

## Support

- **GitHub Repository**: https://github.com/un/webhook-proxy
- **Documentation**: Check the README.md in the repository
- **Issues**: Report issues on the GitHub repository

## Performance Tuning

For high-traffic deployments:

1. **Increase Resources**: Adjust memory and CPU limits in docker-compose.yml
2. **Database Optimization**: Consider using PostgreSQL connection pooling
3. **Redis Configuration**: Tune Redis memory settings based on usage
4. **Load Balancing**: Consider multiple instances behind a load balancer

## Updates

To update the application:

```bash
# Pull latest changes
git pull origin main

# Rebuild and restart
docker-compose -f docker-compose.portainer.yml build --no-cache
docker-compose -f docker-compose.portainer.yml up -d
```
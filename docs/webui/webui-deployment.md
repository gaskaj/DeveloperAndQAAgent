# WebUI Deployment

## Development Setup

### Prerequisites
- Go 1.25+
- Node.js 20+
- PostgreSQL 16+
- Docker + Docker Compose (optional)

### Quick Start with Docker Compose

```bash
docker compose up -d
```

This starts PostgreSQL, the control plane API (port 8080), and the frontend (port 5173).

### Manual Development Setup

1. Start PostgreSQL:
```bash
docker run -d --name oaf-postgres \
  -e POSTGRES_DB=oaf_controlplane \
  -e POSTGRES_USER=oaf \
  -e POSTGRES_PASSWORD=oaf_dev_password \
  -p 5432:5432 \
  postgres:16-alpine
```

2. Build and run the control plane:
```bash
export DB_USER=oaf DB_PASSWORD=oaf_dev_password JWT_SECRET=dev-secret
make build-controlplane
./bin/controlplane --config configs/controlplane.example.yaml
```

3. Run the frontend dev server:
```bash
cd frontend
npm install
npm run dev
```

Frontend runs on http://localhost:5173, proxying API requests to http://localhost:8080.

## Production Deployment

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DB_USER` | Yes | PostgreSQL username |
| `DB_PASSWORD` | Yes | PostgreSQL password |
| `JWT_SECRET` | Yes | Secret for signing JWTs (min 32 chars) |
| `GOOGLE_CLIENT_ID` | No | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | No | Google OAuth client secret |
| `AZURE_TENANT_ID` | No | Azure AD tenant ID |
| `AZURE_CLIENT_ID` | No | Azure AD client ID |
| `AZURE_CLIENT_SECRET` | No | Azure AD client secret |

### Database Migrations

Migrations run automatically on server startup. To run manually:

```bash
./bin/controlplane migrate --config configs/controlplane.yaml
```

### Connecting Agents

1. Register an organization and create an API key in the UI
2. Add control plane config to agent's `config.yaml`:

```yaml
controlplane:
  enabled: true
  url: "https://your-controlplane.example.com"
  api_key: "oaf_your_api_key_here"
  agent_name: "dev-agent-prod-1"
  heartbeat_interval: "30s"
```

3. Restart the agent - it will register and begin reporting events

### Health Check

```
GET /healthz
```

Returns 200 if the server and database connection are healthy.

### Database Configuration for Production

The control plane uses PostgreSQL with advanced connection pooling for optimal performance.

#### Database Connection Configuration

```yaml
database:
  host: "${DB_HOST}"
  port: 5432
  name: "${DB_NAME}"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  sslmode: require  # Use 'require' or 'verify-full' in production
  pool:
    max_connections: 50        # Adjust based on expected load
    min_connections: 10        # Maintain minimum connections
    max_idle_time: "15m"       # Clean up idle connections
    health_check_period: "30s" # Regular health checks
    max_conn_lifetime: "2h"    # Rotate connections periodically
    max_conn_idle_time: "10m"  # Maximum idle time per connection
  performance:
    slow_query_threshold: "100ms" # Log slow queries
    query_timeout: "30s"          # Maximum query time
    enable_query_log: true        # Enable performance logging
    enable_metrics: true          # Enable Prometheus metrics
```

#### Sizing Guidelines

**Development Environment:**
- `max_connections: 10-25`
- `min_connections: 2-5`

**Production Environment:**
- `max_connections: 50-100` (adjust based on concurrent users)
- `min_connections: 10-20` (10-20% of max)

**High-Load Production:**
- `max_connections: 100+`
- Consider read replicas for heavy read workloads

#### Performance Tuning

1. **Monitor Connection Pool Utilization:**
   - Check `/health` endpoint for pool metrics
   - Set up alerts for >80% utilization

2. **Optimize Slow Queries:**
   - Monitor logs for queries exceeding `slow_query_threshold`
   - Add appropriate database indexes
   - Consider query optimization

3. **Database Maintenance:**
   - Regular `VACUUM` and `ANALYZE` operations
   - Monitor database size and growth
   - Set up automated backups

#### Production Database Setup

1. **PostgreSQL Configuration:**
```sql
-- Create dedicated user
CREATE USER oaf_controlplane WITH PASSWORD 'secure_password';

-- Create database
CREATE DATABASE oaf_controlplane OWNER oaf_controlplane;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE oaf_controlplane TO oaf_controlplane;
```

2. **SSL/TLS Configuration:**
   - Use `sslmode: require` or `sslmode: verify-full`
   - Configure PostgreSQL with valid SSL certificates
   - Consider connection encryption in transit

3. **Monitoring and Alerts:**
   - Set up monitoring for database performance
   - Monitor connection pool metrics via `/health`
   - Alert on database connectivity issues

#### Troubleshooting

**Connection Pool Issues:**
- Check logs for "failed to acquire connection" errors
- Monitor pool utilization in health checks
- Increase `max_connections` if consistently high utilization

**Slow Query Performance:**
- Review slow query logs
- Check database indexes on frequently queried columns
- Monitor query execution plans

**Database Connectivity:**
- Verify network connectivity to database
- Check SSL/TLS configuration
- Validate database credentials and permissions

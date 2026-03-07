# Database Optimization Guide

This guide covers database performance optimization, connection pooling, and monitoring best practices for the OpenAgentFramework control plane.

## Table of Contents

1. [Connection Pool Configuration](#connection-pool-configuration)
2. [Query Performance Monitoring](#query-performance-monitoring)
3. [Health Monitoring](#health-monitoring)
4. [Troubleshooting](#troubleshooting)
5. [Best Practices](#best-practices)

## Connection Pool Configuration

The control plane uses pgxpool for connection pooling with PostgreSQL. Proper pool configuration is critical for performance and reliability.

### Basic Configuration

```yaml
database:
  host: localhost
  port: 5432
  name: agentframework
  user: dbuser
  password: "${DB_PASSWORD}"
  sslmode: require
  pool:
    max_connections: 25      # Maximum concurrent connections
    min_connections: 5       # Minimum idle connections
    max_idle_time: "30m"     # Close idle connections after this time
    health_check_period: "1m" # Health check interval
    max_conn_lifetime: "1h"   # Maximum connection lifetime
    max_conn_idle_time: "15m" # Maximum idle time per connection
```

### Production Configuration

For production environments, consider these settings based on your load:

```yaml
database:
  pool:
    max_connections: 50      # Higher for production load
    min_connections: 10      # Ensure minimum availability
    max_idle_time: "15m"     # More aggressive cleanup
    health_check_period: "30s" # More frequent health checks
    max_conn_lifetime: "2h"   # Periodic connection refresh
    max_conn_idle_time: "10m" # Quick idle cleanup
```

### Sizing Guidelines

**Max Connections:**
- Development: 10-25 connections
- Staging: 25-50 connections  
- Production: 50-100 connections
- High-load production: 100+ connections

**Min Connections:**
- Should be 10-20% of max_connections
- Ensure at least 2-5 connections for availability

**Timeouts:**
- `max_idle_time`: 15-30 minutes for production
- `health_check_period`: 30 seconds to 2 minutes
- `max_conn_lifetime`: 1-4 hours to prevent stale connections

## Query Performance Monitoring

The system provides comprehensive query performance monitoring with configurable thresholds.

### Performance Configuration

```yaml
database:
  performance:
    slow_query_threshold: "100ms"  # Log queries slower than this
    query_timeout: "30s"           # Maximum query execution time
    enable_query_log: true         # Log slow queries
    enable_metrics: true           # Collect performance metrics
```

### Slow Query Monitoring

Queries exceeding the `slow_query_threshold` are automatically:

1. **Logged** with execution time and context
2. **Tracked** in Prometheus metrics
3. **Correlated** with request traces

Example slow query log:
```
WARN slow database query sql="SELECT * FROM agents WHERE org_id = $1" duration_ms=150 correlation_id=abc123
```

### Performance Metrics

The system collects these database metrics:

**Query Metrics:**
- `db_queries_total`: Total database operations
- `db_query_duration_ms`: Query execution time distribution
- `db_slow_queries_total`: Count of slow queries
- `db_query_errors_total`: Database error count

**Connection Pool Metrics:**
- `db_pool_total_conns`: Current total connections
- `db_pool_idle_conns`: Current idle connections
- `db_pool_acquired_conns`: Current active connections
- `db_pool_utilization_percent`: Pool utilization percentage

## Health Monitoring

Database health is integrated into the system health checks with detailed diagnostics.

### Health Check Endpoint

The `/health` endpoint includes database status:

```json
{
  "status": "healthy",
  "database": {
    "status": "healthy",
    "response_time": "5ms",
    "query_time": "2ms",
    "pool": {
      "total_conns": 8,
      "idle_conns": 5,
      "acquired_conns": 3,
      "max_conns": 25
    }
  },
  "agents": [...],
  "uptime": "2h15m30s"
}
```

### Health Status Levels

**Healthy:**
- Database responds within 100ms
- Pool has available connections
- No connection errors

**Degraded:**
- Database responds but slowly (>500ms)
- Pool utilization >80%
- Occasional connection timeouts

**Unhealthy:**
- Database connection failures
- Pool exhaustion
- Persistent query timeouts

## Troubleshooting

### Common Issues

#### High Connection Pool Utilization

**Symptoms:**
```
db_pool_utilization_percent > 80
```

**Solutions:**
1. Increase `max_connections`
2. Review slow queries and optimize
3. Check for connection leaks
4. Reduce `max_conn_idle_time`

#### Slow Query Performance

**Symptoms:**
```
db_slow_queries_total increasing
db_query_duration_ms p95 > threshold
```

**Solutions:**
1. Add database indexes for common queries
2. Optimize query patterns
3. Consider query result caching
4. Review database statistics

#### Connection Pool Exhaustion

**Symptoms:**
```
Error: failed to acquire connection from pool
db_pool_acquired_conns = max_conns
```

**Solutions:**
1. Increase `max_connections`
2. Reduce `query_timeout` for stuck queries
3. Check for long-running transactions
4. Review application connection handling

#### Database Connectivity Issues

**Symptoms:**
```
Error: failed to connect to database
Health status: "unhealthy"
```

**Solutions:**
1. Verify database server status
2. Check network connectivity
3. Validate credentials and permissions
4. Review SSL/TLS configuration

### Debugging Tools

#### Enable Debug Logging

```yaml
logging:
  level: debug  # Shows all database operations
```

#### Check Pool Statistics

Use the health endpoint to monitor real-time pool metrics:

```bash
curl -s http://localhost:8080/health | jq '.database.pool'
```

#### Query Performance Analysis

Monitor slow query logs and correlate with application traces:

```bash
grep "slow database query" logs/app.log | tail -20
```

## Best Practices

### Development Environment

1. **Use Local PostgreSQL:** Avoid network latency
2. **Small Pool Size:** 5-10 connections sufficient
3. **Short Timeouts:** Quick feedback on issues
4. **Enable Debug Logging:** See all database operations

```yaml
database:
  pool:
    max_connections: 10
    min_connections: 2
  performance:
    slow_query_threshold: "50ms"
    query_timeout: "10s"
    enable_query_log: true
```

### Production Environment

1. **Size Pool for Load:** Monitor utilization and scale
2. **Aggressive Health Checks:** Quick failure detection
3. **Conservative Timeouts:** Handle network issues gracefully
4. **Monitor Metrics:** Set up alerts on key indicators

```yaml
database:
  pool:
    max_connections: 50
    min_connections: 10
    health_check_period: "30s"
  performance:
    slow_query_threshold: "100ms"
    query_timeout: "30s"
```

### Performance Optimization

1. **Index Common Queries:** Especially org_id lookups
2. **Use Prepared Statements:** Automatic with pgx
3. **Avoid N+1 Queries:** Use JOINs or batch operations
4. **Monitor Query Patterns:** Use slow query logs

### Security Considerations

1. **Use SSL/TLS:** Set `sslmode: require` in production
2. **Rotate Credentials:** Regularly update database passwords
3. **Limit Permissions:** Use dedicated database user with minimal privileges
4. **Network Security:** Use private networks and firewalls

### Monitoring and Alerting

Set up alerts for:

1. **Pool utilization >80%**
2. **Slow query rate increasing**
3. **Database health check failures**
4. **Connection timeout errors**

Example Prometheus alerts:

```yaml
- alert: DatabasePoolUtilization
  expr: db_pool_utilization_percent > 80
  for: 2m
  annotations:
    summary: "Database connection pool utilization high"

- alert: DatabaseSlowQueries
  expr: rate(db_slow_queries_total[5m]) > 0.1
  for: 1m
  annotations:
    summary: "High rate of slow database queries"
```
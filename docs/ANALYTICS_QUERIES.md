# Analytics Queries

This guide provides SQL queries for reporting and analytics against the Nest PostgreSQL database.

## Schema Overview

The key tables for analytics:

| Table | Description | Partitioned |
|-------|-------------|-------------|
| `rule_executions` | One row per rule evaluation (item + rule + verdict) | Yes, by month on `executed_at` |
| `action_executions` | One row per action execution (webhook or MRT enqueue) | Yes, by month on `executed_at` |
| `mrt_jobs` | Manual review jobs with status lifecycle | No |
| `mrt_decisions` | Moderator decisions with verdict and reason | No |
| `rules` | Rule definitions with status, priority, event_types | No |
| `items` | Submitted items ledger | No |
| `orgs` | Organization metadata | No |

Partitioned tables use monthly ranges (e.g., `rule_executions_2026_02`). Always include `executed_at` in WHERE clauses for partition pruning.

## Input Event Counts

### Total items submitted per day

```sql
SELECT
    date_trunc('day', created_at) AS day,
    count(*) AS item_count
FROM items
WHERE org_id = '<org_id>'
  AND created_at >= now() - interval '30 days'
GROUP BY day
ORDER BY day;
```

### Items submitted by item type

```sql
SELECT
    item_type_id,
    count(*) AS item_count
FROM items
WHERE org_id = '<org_id>'
  AND created_at >= now() - interval '7 days'
GROUP BY item_type_id
ORDER BY item_count DESC;
```

### Unique creators per day

```sql
SELECT
    date_trunc('day', created_at) AS day,
    count(DISTINCT creator_id) AS unique_creators
FROM items
WHERE org_id = '<org_id>'
  AND creator_id IS NOT NULL
  AND created_at >= now() - interval '30 days'
GROUP BY day
ORDER BY day;
```

## Verdict Breakdowns

### Verdict distribution (last 24 hours)

```sql
SELECT
    verdict,
    count(*) AS cnt,
    round(100.0 * count(*) / sum(count(*)) OVER (), 1) AS pct
FROM rule_executions
WHERE org_id = '<org_id>'
  AND executed_at >= now() - interval '24 hours'
  AND verdict IS NOT NULL
GROUP BY verdict
ORDER BY cnt DESC;
```

### Verdict trend by day

```sql
SELECT
    date_trunc('day', executed_at) AS day,
    verdict,
    count(*) AS cnt
FROM rule_executions
WHERE org_id = '<org_id>'
  AND executed_at >= now() - interval '30 days'
  AND verdict IS NOT NULL
GROUP BY day, verdict
ORDER BY day, verdict;
```

### Verdict breakdown by rule

```sql
SELECT
    rule_id,
    r.name AS rule_name,
    verdict,
    count(*) AS cnt
FROM rule_executions re
JOIN rules r ON r.id = re.rule_id
WHERE re.org_id = '<org_id>'
  AND re.executed_at >= now() - interval '7 days'
  AND re.verdict IS NOT NULL
GROUP BY rule_id, r.name, verdict
ORDER BY cnt DESC;
```

## MRT Queue Status

### Current queue depths

```sql
SELECT
    q.name AS queue_name,
    j.status,
    count(*) AS job_count
FROM mrt_jobs j
JOIN mrt_queues q ON q.id = j.queue_id
WHERE j.org_id = '<org_id>'
GROUP BY q.name, j.status
ORDER BY q.name, j.status;
```

### Pending jobs per queue (snapshot)

```sql
SELECT
    q.name AS queue_name,
    count(*) AS pending_count
FROM mrt_jobs j
JOIN mrt_queues q ON q.id = j.queue_id
WHERE j.org_id = '<org_id>'
  AND j.status = 'PENDING'
GROUP BY q.name
ORDER BY pending_count DESC;
```

### Average time to assignment (last 7 days)

```sql
SELECT
    q.name AS queue_name,
    avg(j.updated_at - j.created_at) AS avg_time_to_assign
FROM mrt_jobs j
JOIN mrt_queues q ON q.id = j.queue_id
WHERE j.org_id = '<org_id>'
  AND j.status IN ('ASSIGNED', 'DECIDED')
  AND j.created_at >= now() - interval '7 days'
GROUP BY q.name;
```

### Jobs created per day by queue

```sql
SELECT
    date_trunc('day', j.created_at) AS day,
    q.name AS queue_name,
    count(*) AS job_count
FROM mrt_jobs j
JOIN mrt_queues q ON q.id = j.queue_id
WHERE j.org_id = '<org_id>'
  AND j.created_at >= now() - interval '30 days'
GROUP BY day, q.name
ORDER BY day, q.name;
```

## MRT Decisions

### Decisions by moderator (last 7 days)

```sql
SELECT
    u.name AS moderator_name,
    u.email,
    count(*) AS decision_count,
    count(DISTINCT d.job_id) AS unique_jobs
FROM mrt_decisions d
JOIN users u ON u.id = d.user_id
WHERE d.org_id = '<org_id>'
  AND d.created_at >= now() - interval '7 days'
GROUP BY u.name, u.email
ORDER BY decision_count DESC;
```

### Decision verdict distribution

```sql
SELECT
    verdict,
    count(*) AS cnt,
    round(100.0 * count(*) / sum(count(*)) OVER (), 1) AS pct
FROM mrt_decisions
WHERE org_id = '<org_id>'
  AND created_at >= now() - interval '7 days'
GROUP BY verdict
ORDER BY cnt DESC;
```

### Decisions per day with verdict breakdown

```sql
SELECT
    date_trunc('day', created_at) AS day,
    verdict,
    count(*) AS cnt
FROM mrt_decisions
WHERE org_id = '<org_id>'
  AND created_at >= now() - interval '30 days'
GROUP BY day, verdict
ORDER BY day, verdict;
```

### Policy citation frequency in decisions

```sql
SELECT
    p.name AS policy_name,
    count(*) AS citation_count
FROM mrt_decisions d,
     unnest(d.policy_ids) AS pid
JOIN policies p ON p.id = pid
WHERE d.org_id = '<org_id>'
  AND d.created_at >= now() - interval '30 days'
GROUP BY p.name
ORDER BY citation_count DESC;
```

## Rule Hit Rates

### Top rules by execution count (last 7 days)

```sql
SELECT
    re.rule_id,
    r.name AS rule_name,
    r.status,
    count(*) AS execution_count,
    count(*) FILTER (WHERE re.verdict = 'block') AS blocks,
    count(*) FILTER (WHERE re.verdict = 'review') AS reviews,
    count(*) FILTER (WHERE re.verdict = 'approve') AS approves
FROM rule_executions re
JOIN rules r ON r.id = re.rule_id
WHERE re.org_id = '<org_id>'
  AND re.executed_at >= now() - interval '7 days'
GROUP BY re.rule_id, r.name, r.status
ORDER BY execution_count DESC;
```

### Block rate by rule

```sql
SELECT
    re.rule_id,
    r.name AS rule_name,
    count(*) AS total,
    count(*) FILTER (WHERE re.verdict = 'block') AS blocks,
    round(100.0 * count(*) FILTER (WHERE re.verdict = 'block') / NULLIF(count(*), 0), 1) AS block_rate_pct
FROM rule_executions re
JOIN rules r ON r.id = re.rule_id
WHERE re.org_id = '<org_id>'
  AND re.executed_at >= now() - interval '7 days'
  AND re.verdict IS NOT NULL
GROUP BY re.rule_id, r.name
ORDER BY block_rate_pct DESC;
```

### Rules with zero executions (potentially dead rules)

```sql
SELECT
    r.id,
    r.name,
    r.status,
    r.created_at
FROM rules r
LEFT JOIN rule_executions re
    ON re.rule_id = r.id
    AND re.executed_at >= now() - interval '30 days'
WHERE r.org_id = '<org_id>'
  AND r.status = 'LIVE'
  AND re.rule_id IS NULL
ORDER BY r.created_at;
```

## Signal and Action Performance

### Action execution success rates

```sql
SELECT
    a.name AS action_name,
    count(*) AS total_executions,
    count(*) FILTER (WHERE ae.success) AS successes,
    count(*) FILTER (WHERE NOT ae.success) AS failures,
    round(100.0 * count(*) FILTER (WHERE ae.success) / NULLIF(count(*), 0), 1) AS success_rate_pct
FROM action_executions ae
JOIN actions a ON a.id = ae.action_id
WHERE ae.org_id = '<org_id>'
  AND ae.executed_at >= now() - interval '7 days'
GROUP BY a.name
ORDER BY total_executions DESC;
```

### Action executions per day

```sql
SELECT
    date_trunc('day', ae.executed_at) AS day,
    a.name AS action_name,
    count(*) AS cnt,
    count(*) FILTER (WHERE ae.success) AS ok,
    count(*) FILTER (WHERE NOT ae.success) AS fail
FROM action_executions ae
JOIN actions a ON a.id = ae.action_id
WHERE ae.org_id = '<org_id>'
  AND ae.executed_at >= now() - interval '30 days'
GROUP BY day, a.name
ORDER BY day, a.name;
```

### Rule evaluation latency percentiles

```sql
SELECT
    re.rule_id,
    r.name AS rule_name,
    count(*) AS evaluations,
    round(avg(re.latency_us)) AS avg_latency_us,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY re.latency_us) AS p50_us,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY re.latency_us) AS p95_us,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY re.latency_us) AS p99_us,
    max(re.latency_us) AS max_us
FROM rule_executions re
JOIN rules r ON r.id = re.rule_id
WHERE re.org_id = '<org_id>'
  AND re.executed_at >= now() - interval '24 hours'
  AND re.latency_us IS NOT NULL
GROUP BY re.rule_id, r.name
ORDER BY p99_us DESC;
```

## Time-Based Filtering Patterns

### Partition pruning

Always include `executed_at` in WHERE clauses on `rule_executions` and `action_executions`. PostgreSQL will prune irrelevant monthly partitions:

```sql
-- Good: pruned to relevant partitions
WHERE executed_at >= '2026-02-01' AND executed_at < '2026-03-01'

-- Bad: full table scan across all partitions
WHERE date_trunc('month', executed_at) = '2026-02-01'
```

### Common time windows

```sql
-- Last 24 hours
WHERE executed_at >= now() - interval '24 hours'

-- Last 7 days
WHERE executed_at >= now() - interval '7 days'

-- Current month
WHERE executed_at >= date_trunc('month', now())

-- Specific date range
WHERE executed_at >= '2026-02-01' AND executed_at < '2026-02-15'
```

### Hourly bucketing for dashboards

```sql
SELECT
    date_trunc('hour', executed_at) AS hour,
    count(*) AS evaluations,
    count(*) FILTER (WHERE verdict = 'block') AS blocks
FROM rule_executions
WHERE org_id = '<org_id>'
  AND executed_at >= now() - interval '24 hours'
GROUP BY hour
ORDER BY hour;
```

## Entity History

The `entity_history` table stores versioned snapshots of rules, actions, and policies. Use it to audit changes:

### Rule change history

```sql
SELECT
    version,
    valid_from,
    valid_to,
    snapshot->>'name' AS name,
    snapshot->>'status' AS status,
    snapshot->>'source' AS source
FROM entity_history
WHERE entity_type = 'rule'
  AND id = '<rule_id>'
ORDER BY version;
```

### Recent changes across all entity types

```sql
SELECT
    entity_type,
    id,
    version,
    valid_from,
    snapshot->>'name' AS name
FROM entity_history
WHERE org_id = '<org_id>'
  AND valid_from >= now() - interval '7 days'
ORDER BY valid_from DESC
LIMIT 50;
```

-- Nest v1.0 Initial Partitions
-- Monthly partitions for rule_executions and action_executions.
--
-- Coverage: 2026-02 (current month) through 2026-05 (current month + 3 months).
-- The partition maintenance worker (Stage 6, worker/maintenance.go) creates
-- future partitions automatically before each month begins.
--
-- Partition naming convention: <table>_<YYYY>_<MM>
-- Partition bounds: inclusive lower bound, exclusive upper bound (standard RANGE semantics).

-- ============================================================
-- rule_executions partitions
-- ============================================================

CREATE TABLE rule_executions_2026_02 PARTITION OF rule_executions
  FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE rule_executions_2026_03 PARTITION OF rule_executions
  FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE rule_executions_2026_04 PARTITION OF rule_executions
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE rule_executions_2026_05 PARTITION OF rule_executions
  FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

-- ============================================================
-- action_executions partitions
-- ============================================================

CREATE TABLE action_executions_2026_02 PARTITION OF action_executions
  FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE action_executions_2026_03 PARTITION OF action_executions
  FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE action_executions_2026_04 PARTITION OF action_executions
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE action_executions_2026_05 PARTITION OF action_executions
  FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

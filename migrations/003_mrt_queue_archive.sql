-- Nest v1.0 MRT Queue Archive Support
-- Add soft-delete support to mrt_queues via archived_at column.
--
-- The absolute unique constraint on (org_id, name) is replaced with a
-- partial unique index that only applies to non-archived queues. This allows
-- re-creating a queue with the same name after archiving the old one.

ALTER TABLE mrt_queues ADD COLUMN archived_at TIMESTAMPTZ;

-- Drop the auto-generated unique constraint from the inline UNIQUE (org_id, name)
-- declaration in 001_initial.sql.
ALTER TABLE mrt_queues DROP CONSTRAINT IF EXISTS mrt_queues_org_id_name_key;

-- Replace with a partial unique index covering only active (non-archived) queues.
CREATE UNIQUE INDEX mrt_queues_org_id_name_active_key
  ON mrt_queues (org_id, name)
  WHERE archived_at IS NULL;

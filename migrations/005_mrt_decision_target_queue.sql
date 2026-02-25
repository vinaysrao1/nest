ALTER TABLE mrt_decisions ADD COLUMN target_queue_id TEXT REFERENCES mrt_queues(id);

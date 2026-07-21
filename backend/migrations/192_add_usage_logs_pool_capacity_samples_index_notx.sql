-- Build the pool-capacity history index without blocking writes to usage_logs.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_pool_capacity_samples
    ON usage_logs (group_id, created_at DESC, id DESC)
    WHERE group_id IS NOT NULL AND actual_cost > 0 AND request_type <> 4;

-- Transform: hourly-events.sql
-- Purpose: Aggregate user event stream data into hourly engagement metrics.
-- Executed by the Velora-managed Airflow DAG run for 'hourly-events-etl'.
--
-- Expected source: events Kafka topic or Postgres events table
-- Output: MinIO bucket events-warehouse/hourly_metrics/YYYY-MM-DD/HH/

SELECT
    DATE_TRUNC('hour', event_time)                      AS event_hour,
    event_type,
    platform,
    region,
    COUNT(*)                                            AS event_count,
    COUNT(DISTINCT user_id)                             AS unique_users,
    COUNT(DISTINCT session_id)                          AS unique_sessions,
    -- Conversion funnel metrics
    COUNT(*) FILTER (WHERE event_type = 'page_view')    AS page_views,
    COUNT(*) FILTER (WHERE event_type = 'add_to_cart')  AS add_to_cart_events,
    COUNT(*) FILTER (WHERE event_type = 'purchase')     AS purchases,
    -- Conversion rate (purchase / page_view)
    ROUND(
        COUNT(*) FILTER (WHERE event_type = 'purchase')::NUMERIC /
        NULLIF(COUNT(*) FILTER (WHERE event_type = 'page_view'), 0) * 100,
        2
    )                                                   AS conversion_rate_pct,
    -- Avg session duration for completed sessions
    AVG(
        EXTRACT(EPOCH FROM (session_end_time - session_start_time))
    ) FILTER (WHERE session_end_time IS NOT NULL)       AS avg_session_duration_seconds
FROM
    user_events
WHERE
    event_time >= DATE_TRUNC('hour', NOW()) - INTERVAL '1 hour'
    AND event_time <  DATE_TRUNC('hour', NOW())
GROUP BY
    DATE_TRUNC('hour', event_time),
    event_type,
    platform,
    region
ORDER BY
    event_hour,
    event_count DESC;

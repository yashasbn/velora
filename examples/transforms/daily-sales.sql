-- Transform: daily-sales.sql
-- Purpose: Aggregate daily sales metrics from raw transaction data.
-- Executed by the Velora-managed Airflow DAG run for 'daily-sales-etl'.
--
-- Expected source table: raw_transactions (from Postgres sales-db-creds)
-- Output written to MinIO bucket: analytics-warehouse/daily_sales/YYYY-MM-DD/

SELECT
    DATE_TRUNC('day', transaction_time)                     AS sale_date,
    product_id,
    product_category,
    SUM(quantity)                                           AS total_units_sold,
    SUM(unit_price * quantity)                              AS gross_revenue,
    SUM(unit_price * quantity * (1 - discount_rate))        AS net_revenue,
    COUNT(DISTINCT customer_id)                             AS unique_customers,
    COUNT(transaction_id)                                   AS transaction_count,
    AVG(unit_price * quantity)                              AS avg_order_value,
    -- Running 7-day moving average of revenue
    AVG(SUM(unit_price * quantity)) OVER (
        PARTITION BY product_id
        ORDER BY DATE_TRUNC('day', transaction_time)
        ROWS BETWEEN 6 PRECEDING AND CURRENT ROW
    )                                                       AS revenue_7d_avg
FROM
    raw_transactions
WHERE
    transaction_time >= CURRENT_DATE - INTERVAL '1 day'
    AND transaction_time <  CURRENT_DATE
    AND status = 'completed'
GROUP BY
    DATE_TRUNC('day', transaction_time),
    product_id,
    product_category
ORDER BY
    sale_date,
    gross_revenue DESC;

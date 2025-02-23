SELECT source_domain, AVG(amount_in)/1000000 as median_amount
FROM (
SELECT source_domain, amount_in,
       ROW_NUMBER() OVER (PARTITION BY source_domain ORDER BY amount_in) as rn,
       COUNT(*) OVER (PARTITION BY source_domain) as cnt
FROM tx_data
) subquery
WHERE rn IN ((cnt + 1)/2, (cnt + 2)/2)
GROUP BY source_domain

select count(*), filler, source_domain from tx_data where ingestion_timestamp > DATETIME('now', '-1 day') and filler = 'osmo153ly6vgjgk3fvh624a3d0waa53wycyayxl0k4w' group by source_domain;
select count(*), filler, source_domain from tx_data where ingestion_timestamp > DATETIME('now', '-1 day') and filler = 'osmo1xjuvq8mlmhc24l2ewya2uyyj9t6r0dcfdhza6h' group by source_domain;
select count(*), filler, source_domain from tx_data where height >= 30097705 and filler = 'osmo1xjuvq8mlmhc24l2ewya2uyyj9t6r0dcfdhza6h' group by source_domain;
select count(*), filler, source_domain from tx_data where height >= 30097705 and filler = 'osmo153ly6vgjgk3fvh624a3d0waa53wycyayxl0k4w' group by source_domain;

SELECT DATE(b.datetime) as day, SUM(t.solver_revenue)/1000000 as total_revenue FROM tx_data t JOIN osmo_block_times b ON t.height = b.height WHERE t.filler = 'osmo1xjuvq8mlmhc24l2ewya2uyyj9t6r0dcfdhza6h' GROUP BY DATE(b.datetime) HAVING total_revenue > 0 ORDER BY total_revenue DESC;

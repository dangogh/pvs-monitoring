SELECT CAST(received_at / ? AS INTEGER) * ? AS bucket, AVG(solar_kw), AVG(load_kw)
FROM readings WHERE received_at >= ? AND received_at <= ?
GROUP BY bucket ORDER BY bucket

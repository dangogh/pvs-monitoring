SELECT AVG(solar_kw), AVG(load_kw), AVG(net_kw), COUNT(*)
FROM readings WHERE received_at >= ? AND received_at <= ?

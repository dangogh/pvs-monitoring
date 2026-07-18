SELECT ir.serial, ir.state, ir.state_descr, ir.received_at, ir.power_kw, ir.lifetime_kwh,
       ir.voltage_v, ir.current_a, ir.power_mppt1_kw, ir.voltage_mppt1_v, ir.current_mppt1_a,
       ir.temp_c, ir.freq_hz,
       COALESCE(today.today_kwh, 0)
FROM inverter_readings ir
INNER JOIN (SELECT serial, MAX(received_at) AS max_ra FROM inverter_readings GROUP BY serial) latest
        ON ir.serial = latest.serial AND ir.received_at = latest.max_ra
LEFT JOIN (SELECT serial, MAX(lifetime_kwh) - MIN(lifetime_kwh) AS today_kwh
           FROM inverter_readings WHERE received_at >= ? AND lifetime_kwh > 0 GROUP BY serial) today
       ON ir.serial = today.serial
ORDER BY ir.serial

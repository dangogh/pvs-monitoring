INSERT INTO inverter_readings
    (received_at, serial, state, state_descr, power_kw, lifetime_kwh, voltage_v, current_a,
     power_mppt1_kw, voltage_mppt1_v, current_mppt1_a, temp_c, freq_hz)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)

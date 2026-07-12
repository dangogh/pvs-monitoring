INSERT INTO inverter_outages (serial, error_at)
SELECT ?, ? WHERE NOT EXISTS (
    SELECT 1 FROM inverter_outages WHERE serial = ? AND healthy_at IS NULL
)

UPDATE inverter_outages SET healthy_at = ?
WHERE id = (
    SELECT id FROM inverter_outages
    WHERE serial = ? AND healthy_at IS NULL
    ORDER BY error_at DESC LIMIT 1
)

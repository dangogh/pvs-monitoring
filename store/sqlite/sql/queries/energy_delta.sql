-- Energy consumed/produced over a window, from the PVS6 lifetime kWh counters.
--
-- Uses last-minus-first rather than MAX-MINUS-MIN. The counters are supposed to
-- increase monotonically, but firmware faults can make them run backward (see
-- 2026-08-20/21, when both counters inverted for ~29h). Under MAX-MINUS-MIN a
-- declining counter reports the size of the decline as if it were energy, which
-- produced a reading of 128 kWh of household load on a day that actually used 66.
--
-- Summing only the positive step-to-step deltas is NOT a valid alternative: the
-- load counter carries ~3400 small backward steps per day of ordinary jitter, and
-- summing positives inflates a real 79.79 kWh day to 112.44 kWh.
--
-- A negative result means the counter went backward across the window, which is
-- physically impossible; clamp to 0 rather than reporting negative energy.
WITH bounds AS (
    SELECT solar_kwh, load_kwh,
           ROW_NUMBER() OVER (ORDER BY received_at ASC)  AS first_row,
           ROW_NUMBER() OVER (ORDER BY received_at DESC) AS last_row
    FROM readings
    WHERE received_at >= ? AND received_at <= ?
),
delta AS (
    SELECT COALESCE(MAX(CASE WHEN last_row  = 1 THEN solar_kwh END)
                  - MAX(CASE WHEN first_row = 1 THEN solar_kwh END), 0) AS solar,
           COALESCE(MAX(CASE WHEN last_row  = 1 THEN load_kwh  END)
                  - MAX(CASE WHEN first_row = 1 THEN load_kwh  END), 0) AS load
    FROM bounds
)
SELECT CASE WHEN solar > 0 THEN solar ELSE 0 END,
       CASE WHEN load  > 0 THEN load  ELSE 0 END,
       (CASE WHEN load  > 0 THEN load  ELSE 0 END)
     - (CASE WHEN solar > 0 THEN solar ELSE 0 END)
FROM delta

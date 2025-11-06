CREATE TABLE IF NOT EXISTS destination_health (
  destination_id     uuid PRIMARY KEY REFERENCES destination(destination_id) ON DELETE CASCADE,
  window_start       timestamptz NOT NULL DEFAULT now(),
  success_count      integer NOT NULL DEFAULT 0,
  failure_count      integer NOT NULL DEFAULT 0,
  open_until         timestamptz
);


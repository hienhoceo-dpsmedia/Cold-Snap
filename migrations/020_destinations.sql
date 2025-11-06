CREATE TABLE IF NOT EXISTS destination (
  destination_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name               text NOT NULL UNIQUE,
  url                text NOT NULL,
  headers            jsonb NOT NULL DEFAULT '{}',
  secret             text,
  connect_timeout_s  integer NOT NULL DEFAULT 5,
  timeout_s          integer NOT NULL DEFAULT 15,
  verify_tls         boolean NOT NULL DEFAULT true,

  max_rps            numeric(10,3) NOT NULL DEFAULT 5.0,
  burst              integer NOT NULL DEFAULT 10,
  max_inflight       integer NOT NULL DEFAULT 5,

  breaker_failure_ratio numeric(5,2) NOT NULL DEFAULT 0.5,
  breaker_min_requests  integer NOT NULL DEFAULT 20,
  breaker_cooldown_s    integer NOT NULL DEFAULT 60,

  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS destination_name_idx ON destination(name);


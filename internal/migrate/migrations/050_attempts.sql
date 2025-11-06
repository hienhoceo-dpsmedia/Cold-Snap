DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'attempt_status') THEN
    CREATE TYPE attempt_status AS ENUM ('pending','picked','succeeded','failed');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS delivery_attempt (
  attempt_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id           uuid NOT NULL REFERENCES event(event_id) ON DELETE CASCADE,
  route_id           uuid NOT NULL REFERENCES route(route_id) ON DELETE CASCADE,
  attempt_no         integer NOT NULL DEFAULT 0,
  status             attempt_status NOT NULL DEFAULT 'pending',
  next_at            timestamptz NOT NULL DEFAULT now(),
  picked_at          timestamptz,
  succeeded_at       timestamptz,
  failed_at          timestamptz,

  http_code          integer,
  response_headers   jsonb,
  response_body      text,
  response_error     text,
  elapsed_ms         integer,

  worker_name        text,
  worker_version     text,

  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS attempt_sched_idx ON delivery_attempt (next_at, status);
CREATE INDEX IF NOT EXISTS attempt_route_status_idx ON delivery_attempt (route_id, status, next_at);
CREATE INDEX IF NOT EXISTS attempt_route_order_idx ON delivery_attempt (route_id, next_at);


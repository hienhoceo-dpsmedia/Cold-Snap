CREATE TABLE IF NOT EXISTS event (
  event_id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id          uuid NOT NULL REFERENCES source(source_id) ON DELETE CASCADE,
  received_at        timestamptz NOT NULL DEFAULT now(),
  content_type       text,
  headers            jsonb NOT NULL DEFAULT '{}',
  body               bytea NOT NULL,
  body_size          integer NOT NULL,
  source_ip          inet,
  idempotency_key    text,
  body_hash_sha256   bytea
);
CREATE UNIQUE INDEX IF NOT EXISTS event_source_idempotency_key_idx ON event(source_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS event_source_received_idx ON event(source_id, received_at DESC);


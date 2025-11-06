CREATE TABLE IF NOT EXISTS source (
  source_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name               text NOT NULL UNIQUE,
  token              text NOT NULL UNIQUE,
  enabled            boolean NOT NULL DEFAULT true,
  ip_allow_cidrs     text[] DEFAULT '{}',
  max_body_bytes     integer NOT NULL DEFAULT 1048576,
  created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS source_enabled_idx ON source(enabled);


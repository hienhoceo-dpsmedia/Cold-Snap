CREATE TABLE IF NOT EXISTS route (
  route_id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id          uuid NOT NULL REFERENCES source(source_id) ON DELETE CASCADE,
  destination_id     uuid NOT NULL REFERENCES destination(destination_id) ON DELETE CASCADE,
  enabled            boolean NOT NULL DEFAULT true,

  content_type_like  text,
  json_path          text,
  json_equals        text,

  rate_override      jsonb NOT NULL DEFAULT '{}',

  UNIQUE(source_id, destination_id)
);
CREATE INDEX IF NOT EXISTS route_enabled_idx ON route(enabled);
CREATE INDEX IF NOT EXISTS route_source_idx ON route(source_id);


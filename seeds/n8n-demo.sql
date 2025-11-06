-- Demo config to front n8n with rate-limited gateway
-- Assumes an n8n container reachable at http://n8n:5678

INSERT INTO source (name, token)
VALUES ('n8n-source', 'n8n_token')
ON CONFLICT (name) DO UPDATE SET token=EXCLUDED.token;

INSERT INTO destination (name, url, headers, secret, max_rps, burst, max_inflight, append_path)
VALUES ('n8n', 'http://n8n:5678', '{}', NULL, 5.0, 5, 3, true)
ON CONFLICT (name) DO UPDATE SET url=EXCLUDED.url;

INSERT INTO route (source_id, destination_id, enabled, ord)
SELECT s.source_id, d.destination_id, true, 0
FROM source s, destination d
WHERE s.name='n8n-source' AND d.name='n8n'
ON CONFLICT (source_id, destination_id) DO NOTHING;


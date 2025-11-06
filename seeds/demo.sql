-- Create a demo source, destination, and route
INSERT INTO source (name, token)
VALUES ('demo-source', 'demo_token')
ON CONFLICT (name) DO UPDATE SET token=EXCLUDED.token;

INSERT INTO destination (name, url, headers, secret, max_rps, burst, max_inflight, append_path)
VALUES ('httpbin', 'https://httpbin.org', '{"X-Static": "1"}', NULL, 5.0, 5, 3, true)
ON CONFLICT (name) DO UPDATE SET url=EXCLUDED.url;

INSERT INTO route (source_id, destination_id, enabled, content_type_like, ord)
SELECT s.source_id, d.destination_id, true, 'application/json%', 0
FROM source s, destination d
WHERE s.name='demo-source' AND d.name='httpbin'
ON CONFLICT (source_id, destination_id) DO NOTHING;


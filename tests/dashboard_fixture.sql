BEGIN;

INSERT OR REPLACE INTO providerConnections
  (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
VALUES
  ('fixture-openai', 'openai', 'apikey', 'OpenAI / Primary', 'fixture-openai@example.invalid', 1, 1, '{"baseUrl":"https://api.openai.invalid/v1"}', datetime('now'), datetime('now')),
  ('fixture-anthropic', 'anthropic', 'apikey', 'Anthropic / Fallback', 'fixture-anthropic@example.invalid', 2, 1, '{"baseUrl":"https://api.anthropic.invalid"}', datetime('now'), datetime('now'));

INSERT OR REPLACE INTO apiKeys
  (id, key, name, machineId, isActive, restrictions, createdAt)
VALUES
  ('fixture-key-readonly', 'sk-zy-fixture-readonly', 'Dashboard fixture key', 'fixture-machine', 1, '{"allowedPrefixes":["gpt-*","claude-*"],"allowedProviders":["fixture-openai","fixture-anthropic"]}', datetime('now'));

INSERT OR REPLACE INTO combos
  (id, name, kind, models, strategy, createdAt, updatedAt)
VALUES
  ('fixture-combo-primary', 'Fixture fallback route', 'text', '["gpt-4o-mini","claude-3-5-sonnet"]', 'fallback', datetime('now'), datetime('now'));

INSERT OR REPLACE INTO settings (id, data)
VALUES (1, '{"fixture":true,"source":"tests/dashboard_fixture.sql"}');

DELETE FROM usageHistory WHERE apiKey = 'sk-zy-fixture-readonly';
INSERT INTO usageHistory
  (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
VALUES
  (datetime('now', '-1 day'), 'openai', 'gpt-4o-mini', 'fixture-openai', 'sk-zy-fixture-readonly', '/chat/completions', 420, 180, 0.0021, '200', '{"total":600}', '{"fixture":true}'),
  (datetime('now'), 'anthropic', 'claude-3-5-sonnet', 'fixture-anthropic', 'sk-zy-fixture-readonly', '/messages', 310, 240, 0.0048, '200', '{"total":550}', '{"fixture":true}');

COMMIT;

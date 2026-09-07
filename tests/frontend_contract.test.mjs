import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const app = await readFile(new URL('../frontend/app.js', import.meta.url), 'utf8');
const html = await readFile(new URL('../frontend/index.html', import.meta.url), 'utf8');

for (const endpoint of [
  '/api/providers', '/api/combos', '/api/keys', '/api/settings',
  '/api/proxy-pools', '/api/model-aliases', '/api/admin/model-policy/preview', '/api/admin/security/summary', '/admin/health/reset', '/usage/stream', '/translator/console-logs', '/translator/console-logs/stream',
  '/models', '/chat/completions'
]) {
  assert.match(app, new RegExp(endpoint.replace('/', '\/')), `missing frontend endpoint: ${endpoint}`);
}

assert.match(app, /Authorization: `Bearer \$\{apiKey\}`/);
assert.match(app, /async function copyText\(value\)/, 'clipboard compatibility helper is required');
assert.match(app, /document\.execCommand\('copy'\)/, 'clipboard HTTP fallback is required');
assert.match(app, /create-only secret/, 'API key list must mark secrets as create-only');
assert.match(app, /function showOneTimeKeyModal\(key\)/, 'new keys must use a one-time secret flow');
assert.match(app, /btn-fetch-alias-models/, 'admin model alias form must keep the provider fetch helper');
assert.match(app, /alias-only/i, 'frontend contract must document alias-only model access');
assert.doesNotMatch(app, /Edit Prefix/, 'provider prefix must not be exposed as a client-facing management action');
assert.match(app, /data-key-scope/, 'API key governance must expose scope filters');
assert.match(app, /Public Model Alias/, 'combo management must expose a public alias field');
assert.match(app, /Internal Upstream Inventory/i, 'provider management must distinguish private inventory');
assert.match(app, /btn-preview-key-policy/, 'API key policy builder must expose authorization preview');
assert.match(app, /const values = Object\.fromEntries\(new FormData\(form\)\.entries\(\)\);[\s\S]*?submitBtn\.disabled = true;/,
  'deployment form values must be captured before controls are disabled');
assert.match(app, /Public\/no-auth providers have no providerConnections row/,
  'policy builder must include active public providers without connection rows');
assert.match(html, /id="generic-content"/);
assert.match(html, /Issue Gateway API Key/, 'admin shortcut must describe gateway keys, not a client dashboard');
assert.doesNotMatch(html, /:3840/, 'dashboard must not show a stale hardcoded engine port');
console.log('frontend backend contract checks passed');

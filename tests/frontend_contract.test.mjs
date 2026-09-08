import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const app = await readFile(new URL('../frontend/app.js', import.meta.url), 'utf8');
const html = await readFile(new URL('../frontend/index.html', import.meta.url), 'utf8');

const declaredViews = new Set([...html.matchAll(/data-view="([^"]+)"/g)].map((match) => match[1]));
for (const view of declaredViews) {
  const key = view.includes('-') ? `'${view}'` : view;
  assert.match(app, new RegExp(`${key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*:`), `missing view registry entry: ${view}`);
}

for (const endpoint of [
  '/api/providers', '/api/combos', '/api/keys', '/api/settings',
  '/api/proxy-pools', '/api/model-aliases', '/api/admin/model-policy/preview', '/api/admin/security/summary', '/admin/health/reset', '/usage/stream', '/translator/console-logs', '/translator/console-logs/stream',
  '/api/admin/account-types', '/api/admin/users',
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
assert.match(app, /telegramUserId|Telegram Identity/, 'API key table must expose verified Telegram identity metadata');
assert.match(app, /data-key-page/, 'API key table must expose pagination controls');
assert.match(app, /Public Model Alias/, 'combo management must expose a public alias field');
assert.match(app, /published aliases or valid provider\/model targets/, 'combo builder must restrict steps to published aliases or admin-internal targets');
assert.match(app, /Internal Upstream Inventory/i, 'provider management must distinguish private inventory');
assert.match(app, /PUBLIC: ALIAS-ONLY/, 'provider catalog must communicate alias-only public exposure');
assert.match(app, /btn-preview-key-policy/, 'API key policy builder must expose authorization preview');
assert.match(app, /const values = Object\.fromEntries\(new FormData\(form\)\.entries\(\)\);[\s\S]*?submitBtn\.disabled = true;/,
  'deployment form values must be captured before controls are disabled');
assert.match(app, /Public\/no-auth providers have no providerConnections row/,
  'policy builder must include active public providers without connection rows');
assert.match(html, /id="generic-content"/);
assert.match(html, /app\.js\?v=2\.8\.0/, 'frontend asset version must be bumped after dashboard changes');
assert.match(html, /Issue Gateway API Key/, 'admin shortcut must describe gateway keys, not a client dashboard');
assert.doesNotMatch(html, /:3840/, 'dashboard must not show a stale hardcoded engine port');
assert.match(html, /data-view="account-types"/, 'sidebar must include Account Types navigation button');
assert.match(app, /renderAccountTypes/, 'frontend must include Account Types view renderer');
assert.match(app, /Tier-Based Model Governance Active/, 'API key form must document tier-based model governance');
assert.match(app, /manage-tier-models/, 'account types must support managing model permissions per tier');
assert.match(app, /name === 'account-types'[\s\S]*?openTierModal\(null\)/, 'Account Types header action must open the tier editor');
assert.match(app, /published aliases or valid provider\/model targets/, 'combo submission must validate public aliases and internal targets');
assert.match(app, /kind: 'composite'/, 'combo creation must use the unified composite alias endpoint');
assert.match(app, /members: finalModels/, 'composite alias must submit internal routing members with one public alias');
assert.match(app, /btn-fetch-combo-models/, 'combo editor must provide an admin-only upstream model fetch action');
assert.match(app, /btn-add-combo-target/, 'combo editor must provide an explicit internal target action');
assert.match(app, /nodesPayload\.nodes/, 'combo provider picker must resolve friendly custom-node metadata');
assert.match(app, /data-test-model/, 'admin provider inventory must retain a dedicated upstream test action');
assert.match(app, /\/api\/providers\/\$\{encodeURIComponent\(provId\)\}\/test-model/, 'upstream model tests must use the admin provider test endpoint');
assert.match(app, /data-quick-alias/, 'internal inventory must provide an explicit publish-alias action');
assert.match(app, /UNPUBLISHED/, 'internal provider inventory must show unpublished models before alias publication');
assert.match(app, /data-filter-pool-status/, 'proxy pools must expose status filters');
assert.match(app, /providerFetchedModelsCache\.set/, 'provider fetch results must stay in the private admin inventory cache');
assert.match(app, /sourceCounts/, 'provider inventory UI must distinguish upstream, catalog, and custom model sources');
assert.match(app, /function formatWIBTimestamp/, 'dashboard timestamps must use explicit WIB formatting');
assert.match(app, /const\s+timeStr\s*=\s*formatWIBTimestamp\(topReq\.timestamp\);/, 'bindLogStream must format topReq timestamp without ReferenceError');
assert.match(app, /emptyRow\.remove\(\)/, 'bindLogStream must remove empty row when new live entries arrive');
assert.match(app, /ensureGlobalStream\(\)/, 'dashboard must start realtime SSE after authentication');
assert.doesNotMatch(app, /Successfully imported .* models from upstream/, 'provider fetch must not auto-create public/custom model records');
console.log('frontend backend contract checks passed');


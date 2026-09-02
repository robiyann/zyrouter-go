import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const app = await readFile(new URL('../frontend/app.js', import.meta.url), 'utf8');
const html = await readFile(new URL('../frontend/index.html', import.meta.url), 'utf8');

for (const endpoint of [
  '/api/providers', '/api/combos', '/api/keys', '/api/settings',
  '/api/proxy-pools', '/api/model-aliases', '/admin/health/reset', '/usage/stream', '/translator/console-logs', '/translator/console-logs/stream',
  '/models', '/chat/completions'
]) {
  assert.match(app, new RegExp(endpoint.replace('/', '\/')), `missing frontend endpoint: ${endpoint}`);
}

assert.match(app, /Authorization: `Bearer \$\{apiKey\}`/);
assert.match(app, /const values = Object\.fromEntries\(new FormData\(form\)\.entries\(\)\);[\s\S]*?submitBtn\.disabled = true;/,
  'deployment form values must be captured before controls are disabled');
assert.match(html, /id="generic-content"/);
console.log('frontend backend contract checks passed');

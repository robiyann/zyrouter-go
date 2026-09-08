import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'client-portal');
const app = await readFile(path.join(root, 'app.js'), 'utf8');
const html = await readFile(path.join(root, 'index.html'), 'utf8');
const readme = await readFile(path.join(root, 'README.md'), 'utf8');

assert.doesNotMatch(app, /3840/, 'client must not reference the retired local port');
assert.doesNotMatch(readme, /3840/, 'client documentation must not reference the retired local port');
assert.match(app, /\/api\/user\/logs\/stream/, 'client must connect to the private user log stream');
assert.match(app, /EventSource\(url, \{ withCredentials: true \}\)/, 'SSE must send the HttpOnly user cookie');
assert.match(app, /bootstrapAttempt/, 'stale initial bootstrap must not hide a newly verified dashboard');
assert.match(app, /timeLeftSec = 600/, 'verification code must remain valid for ten minutes');
assert.match(app, /INFERENCE_BASE.*v1\/chat\/completions|INFERENCE_BASE\}\/v1\/chat\/completions/, 'inference must use the API domain separately');
assert.doesNotMatch(app, /localStorage\.setItem\(['"]zy_client_session/, 'user session must not be stored in localStorage');
assert.match(html, /data-tab="logs"/, 'client portal must expose the private live logs tab');
assert.match(html, /id="playground-api-key"/, 'playground must support one-time in-memory key entry after reload');
assert.match(html, /id="challenge-timer">10:00/, 'verification timer must start at ten minutes');
console.log('Client frontend contract passed.');

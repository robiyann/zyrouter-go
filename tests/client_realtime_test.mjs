import http from 'node:http';
import { spawn } from 'node:child_process';
import { rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const backendDir = path.join(rootDir, 'backend');
const dbPath = path.join(__dirname, 'client_realtime_test.sqlite');
const port = 21400;
const mockPort = 21401;
const adminPassword = 'client-realtime-admin-pass';
const webhookSecret = 'client-realtime-tg-secret';

const mock = http.createServer(async (req, res) => {
  if (req.method !== 'POST') {
    res.writeHead(404); res.end(); return;
  }
  for await (const _ of req) { /* consume body */ }
  res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
  res.write('data: {"id":"mock-1","choices":[{"delta":{"content":"client realtime ok"}}]}\n\n');
  res.write('data: {"id":"mock-1","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}\n\n');
  res.end('data: [DONE]\n\n');
});
await new Promise((resolve) => mock.listen(mockPort, '127.0.0.1', resolve));

const binPath = path.join(backendDir, 'zyrouter.exe');
for (const suffix of ['', '-wal', '-shm']) if (existsSync(dbPath + suffix)) await rm(dbPath + suffix, { force: true });
const build = spawn('go', ['build', '-trimpath', '-o', 'zyrouter.exe', './cmd/zyrouter'], { cwd: backendDir, stdio: 'inherit', shell: true });
await new Promise((resolve, reject) => build.on('close', (code) => code === 0 ? resolve() : reject(new Error(`build failed: ${code}`))));
const server = spawn(binPath, [], {
  cwd: backendDir,
  env: { ...process.env, HOST: '127.0.0.1', PORT: String(port), DB_PATH: dbPath, INITIAL_PASSWORD: adminPassword, TELEGRAM_WEBHOOK_SECRET: webhookSecret },
  stdio: ['ignore', 'pipe', 'pipe'],
});
const base = `http://127.0.0.1:${port}`;
async function waitReady() {
  for (let i = 0; i < 50; i++) {
    try { if ((await fetch(`${base}/health`)).ok) return; } catch {}
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error('server did not start');
}
function cookieOf(response) { return response.headers.get('set-cookie')?.split(';')[0] || ''; }
async function jsonRequest(url, options = {}) {
  const response = await fetch(`${base}${url}`, { ...options, headers: { 'Content-Type': 'application/json', ...(options.headers || {}) } });
  return { response, body: await response.json().catch(() => ({})) };
}

try {
  await waitReady();
  const login = await jsonRequest('/api/auth/login', { method: 'POST', body: JSON.stringify({ password: adminPassword }) });
  assert.equal(login.response.status, 200);
  const adminCookie = cookieOf(login.response);
  const adminHeaders = { Cookie: adminCookie };

  const provider = await jsonRequest('/api/providers', {
    method: 'POST', headers: adminHeaders,
    body: JSON.stringify({ provider: 'openai', name: 'Client Realtime Mock', authType: 'apikey', data: JSON.stringify({ apiKey: 'mock', baseUrl: `http://127.0.0.1:${mockPort}` }) }),
  });
  assert.equal(provider.response.status, 201);
  const alias = await jsonRequest('/api/model-aliases', {
    method: 'POST', headers: adminHeaders,
    body: JSON.stringify({ alias: 'client-realtime', provider: 'openai', upstreamModel: 'mock-client-model', target: 'openai/mock-client-model', isActive: 1 }),
  });
  assert.ok([200, 201].includes(alias.response.status));
  const policy = await jsonRequest('/api/admin/account-types/user/models', {
    method: 'POST', headers: adminHeaders, body: JSON.stringify({ aliases: ['client-realtime'] }),
  });
  assert.equal(policy.response.status, 200);

  const start = await jsonRequest('/api/user/verification/start', { method: 'POST' });
  const challengeId = start.body.challengeId;
  const verificationCookie = cookieOf(start.response);
  await jsonRequest('/api/telegram/webhook', {
    method: 'POST', headers: { 'X-Telegram-Bot-Api-Secret-Token': webhookSecret },
    body: JSON.stringify({ message: { from: { id: 9001, username: 'realtime_user', first_name: 'Realtime' }, text: `/start ${challengeId}` } }),
  });
  const verified = await fetch(`${base}/api/user/verification/${challengeId}`, { headers: { Cookie: verificationCookie } });
  assert.equal(verified.status, 200);
  const userCookie = cookieOf(verified);
  const userHeaders = { Cookie: userCookie };
  const keyResponse = await jsonRequest('/api/user/key', { method: 'POST', headers: userHeaders, body: '{}' });
  assert.equal(keyResponse.response.status, 201);

  const stream = await fetch(`${base}/api/user/logs/stream`, { headers: userHeaders });
  assert.equal(stream.status, 200);
  const reader = stream.body.getReader();
  const decoder = new TextDecoder();
  let streamText = decoder.decode((await reader.read()).value);
  assert.match(streamText, /event: snapshot/);

  const inference = await fetch(`${base}/v1/chat/completions`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${keyResponse.body.key}` },
    body: JSON.stringify({ model: 'client-realtime', messages: [{ role: 'user', content: 'hello' }], stream: true }),
  });
  assert.equal(inference.status, 200);
  await inference.text();

  const deadline = Date.now() + 3000;
  while (!streamText.includes('request.completed') && Date.now() < deadline) {
    const result = await Promise.race([
      reader.read(),
      new Promise((resolve) => setTimeout(() => resolve({ timeout: true }), 100)),
    ]);
    if (!result.timeout && result.value) streamText += decoder.decode(result.value);
  }
  assert.match(streamText, /request\.completed/);
  assert.match(streamText, /client-realtime/);
  assert.doesNotMatch(streamText, /mock-client-model|openai/);

  const history = await fetch(`${base}/api/user/logs?limit=10`, { headers: userHeaders });
  const historyBody = await history.json();
  assert.equal(history.status, 200);
  assert.equal(historyBody.items[0].model, 'client-realtime');
  assert.doesNotMatch(JSON.stringify(historyBody), /mock-client-model|openai/);

  const blockedAdmin = await fetch(`${base}/api/providers`, { headers: userHeaders });
  assert.equal(blockedAdmin.status, 401);
  console.log('PASS: authenticated client request produced private realtime SSE logs and sanitized history');
} finally {
  server.kill('SIGKILL');
  mock.close();
  for (const suffix of ['', '-wal', '-shm']) if (existsSync(dbPath + suffix)) await rm(dbPath + suffix, { force: true }).catch(() => {});
}

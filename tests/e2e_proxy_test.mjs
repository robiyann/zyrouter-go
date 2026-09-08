import http from 'node:http';
import { spawn } from 'node:child_process';
import { rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const rootDir = path.resolve(__dirname, '..');
const backendDir = path.join(rootDir, 'backend');
const dbPath = path.join(__dirname, 'e2e_test.sqlite');
const testPort = 21398;
const testHost = '127.0.0.1';
const adminPassword = 'e2e-secret-pass-2026';

console.log('--- Starting Zyrouter E2E Proxy & Governance Integration Test ---');

// 1. Setup Mock Upstream Server
let mockCalls = 0;
const mockServer = http.createServer((req, res) => {
  mockCalls++;
  let body = '';
  req.on('data', (chunk) => { body += chunk; });
  req.on('end', () => {
    let parsedBody = {};
    try { parsedBody = JSON.parse(body); } catch {}

    if (req.method === 'POST' && (req.url === '/' || req.url?.includes('chat/completions'))) {
      if (parsedBody.stream) {
        res.writeHead(200, {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
          'Connection': 'keep-alive',
        });
        res.write('data: {"id":"chatcmpl-e2e","choices":[{"delta":{"content":"Zyrouter "}}]}\n\n');
        res.write('data: {"id":"chatcmpl-e2e","choices":[{"delta":{"content":"E2E Stream OK!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}\n\n');
        res.write('data: [DONE]\n\n');
        res.end();
      } else {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          id: 'chatcmpl-e2e-nonstream',
          object: 'chat.completion',
          choices: [{
            index: 0,
            message: { role: 'assistant', content: 'Zyrouter E2E Non-Stream OK!' },
            finish_reason: 'stop',
          }],
          usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
        }));
      }
      return;
    }

    if (req.url?.includes('/v1/models') || req.url?.includes('/models')) {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        data: [{ id: 'mock-raw-gpt' }],
        object: 'list',
      }));
      return;
    }

    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'not found' }));
  });
});

const mockPort = await new Promise((resolve) => {
  mockServer.listen(0, '127.0.0.1', () => {
    const addr = mockServer.address();
    resolve(typeof addr === 'object' && addr ? addr.port : 21990);
  });
});
console.log(`[✓] Mock Upstream Server running on http://127.0.0.1:${mockPort}`);

// 2. Clean previous test database
for (const suffix of ['', '-wal', '-shm']) {
  if (existsSync(dbPath + suffix)) {
    await rm(dbPath + suffix, { force: true });
  }
}

// 3. Ensure binary is available
const binPath = path.join(backendDir, 'zyrouter.exe');
if (!existsSync(binPath)) {
  console.log('[*] Building backend binary...');
  const build = spawn('go', ['build', '-trimpath', '-o', 'zyrouter.exe', './cmd/zyrouter'], {
    cwd: backendDir,
    stdio: 'inherit',
    shell: true,
  });
  await new Promise((res, rej) => {
    build.on('close', (code) => (code === 0 ? res() : rej(new Error(`go build failed with code ${code}`))));
  });
}

// 4. Spawn Zyrouter process
const proxyProcess = spawn(binPath, [], {
  cwd: backendDir,
  env: {
    ...process.env,
    HOST: testHost,
    PORT: String(testPort),
    DB_PATH: dbPath,
    INITIAL_PASSWORD: adminPassword,
    FRONTEND_DIR: path.join(rootDir, 'frontend'),
  },
  stdio: ['ignore', 'pipe', 'pipe'],
});

proxyProcess.stderr.on('data', (d) => {
  const msg = d.toString().trim();
  if (msg.includes('error') || msg.includes('FATAL')) {
    console.error(`[zyrouter err] ${msg}`);
  }
});

// Helper: poll until proxy is responsive
const baseUrl = `http://${testHost}:${testPort}`;
async function waitForReady(maxAttempts = 40) {
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const res = await fetch(`${baseUrl}/health`);
      if (res.ok) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('Zyrouter failed to start in time');
}

let cookieHeader = '';
try {
  await waitForReady();
  console.log(`[✓] Zyrouter Server ready at ${baseUrl}`);

  // Scenario 1: Model discovery starts empty (Alias-Only Invariant)
  {
    console.log('[TEST 1] Testing initial /models endpoint...');
    const res = await fetch(`${baseUrl}/models`);
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.deepEqual(body.data, [], 'Initial /models must return empty list before aliases are published');
    console.log('  -> PASS: initial /models returns 0 published models');
  }

  // Scenario 2: Admin Login
  {
    console.log('[TEST 2] Testing admin login & session...');
    const res = await fetch(`${baseUrl}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: adminPassword }),
    });
    assert.equal(res.status, 200);
    const setCookie = res.headers.get('set-cookie');
    assert.ok(setCookie, 'Login must set auth_token cookie');
    cookieHeader = setCookie.split(';')[0];
    console.log('  -> PASS: admin login successful');
  }

  // Scenario 3: Create Provider Connection pointing to Mock Upstream
  let connId = '';
  {
    console.log('[TEST 3] Creating mock provider connection...');
    const res = await fetch(`${baseUrl}/api/providers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        provider: 'openai',
        name: 'Mock Upstream Node',
        authType: 'apikey',
        data: JSON.stringify({ apiKey: 'sk-mock-key', baseUrl: `http://127.0.0.1:${mockPort}` }),
      }),
    });
    assert.equal(res.status, 201);
    const conn = await res.json();
    connId = conn.id;
    assert.ok(connId, 'Provider connection ID must be generated');
    console.log(`  -> PASS: provider connection created (id: ${connId})`);
  }

  // Scenario 4: Fail-closed prefix & unaliased model rejection
  {
    console.log('[TEST 4] Verifying provider-prefix & unaliased model rejection...');
    // Direct provider prefix request
    const prefixRes = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        model: 'openai/mock-raw-gpt',
        messages: [{ role: 'user', content: 'test' }],
      }),
    });
    assert.equal(prefixRes.status, 403, 'Provider prefix request must be rejected fail-closed (403)');
    const prefixBody = await prefixRes.text();
    assert.ok(prefixBody.includes('provider_prefix_forbidden'), 'Expected provider_prefix_forbidden error');

    // Unaliased raw upstream model request
    const rawRes = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        model: 'mock-raw-gpt',
        messages: [{ role: 'user', content: 'test' }],
      }),
    });
    assert.equal(rawRes.status, 403, 'Unaliased model request must be rejected (403)');
    const rawBody = await rawRes.text();
    assert.ok(rawBody.includes('model_alias_required'), 'Expected model_alias_required error');
    console.log('  -> PASS: prefix and unaliased requests rejected fail-closed (403 Forbidden)');
  }

  // Scenario 5: Publish Model Alias
  {
    console.log('[TEST 5] Publishing bare model alias...');
    const res = await fetch(`${baseUrl}/api/model-aliases`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        alias: 'fast-llm',
        provider: 'openai',
        upstreamModel: 'mock-raw-gpt',
        target: 'openai/mock-raw-gpt',
        isActive: 1,
      }),
    });
    assert.equal(res.status, 200);

    // Verify /models now exposes only the published bare alias
    const mRes = await fetch(`${baseUrl}/models`);
    const mBody = await mRes.json();
    assert.equal(mBody.data.length, 1);
    assert.equal(mBody.data[0].id, 'fast-llm');
    console.log('  -> PASS: published model alias fast-llm exposed at /models');
  }

  // Scenario 6: Combo Orchestration
  {
    console.log('[TEST 6] Creating combo pipeline and published alias...');
    const comboRes = await fetch(`${baseUrl}/api/combos`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        name: 'fallback-pipeline',
        strategy: 'fallback',
        models: JSON.stringify(['fast-llm']),
      }),
    });
    assert.equal(comboRes.status, 201);

    // Register alias for the combo
    const aliasRes = await fetch(`${baseUrl}/api/model-aliases`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        alias: 'smart-combo',
        target: 'combo:fallback-pipeline',
        isActive: 1,
      }),
    });
    assert.equal(aliasRes.status, 200);

    const mRes = await fetch(`${baseUrl}/models`);
    const mBody = await mRes.json();
    const ids = mBody.data.map((m) => m.id).sort();
    assert.deepEqual(ids, ['fast-llm', 'smart-combo']);
    console.log('  -> PASS: combo created and exposed via alias smart-combo');
  }

  // Scenario 7: Create Gateway Key with Granular Restrictions
  let gatewayKey = '';
  {
    console.log('[TEST 7] Creating gateway API key with model whitelist...');
    const res = await fetch(`${baseUrl}/api/keys`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        name: 'e2e-restricted-key',
        restrictions: { allowedModels: ['fast-llm', 'smart-combo'] },
      }),
    });
    assert.equal(res.status, 201);
    const keyData = await res.json();
    gatewayKey = keyData.key;
    assert.ok(gatewayKey.startsWith('zy_'), 'API key must start with zy_ prefix');
    console.log('  -> PASS: restricted API key generated');
  }

  // Scenario 8: Streaming Chat Request via Gateway Key
  {
    console.log('[TEST 8] Executing streaming chat completion with gateway key...');
    const res = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${gatewayKey}`,
      },
      body: JSON.stringify({
        model: 'fast-llm',
        messages: [{ role: 'user', content: 'Say hello' }],
        stream: true,
      }),
    });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('content-type'), 'text/event-stream');
    const text = await res.text();
    assert.ok(text.includes('Zyrouter '), 'Stream chunk 1 missing');
    assert.ok(text.includes('E2E Stream OK!'), 'Stream chunk 2 missing');
    assert.ok(text.includes('[DONE]'), 'Stream end missing');
    console.log('  -> PASS: SSE stream correctly passed through and recorded');
  }

  // Scenario 9: Non-Streaming Chat Request via Combo Route
  {
    console.log('[TEST 9] Executing non-streaming chat completion via combo alias...');
    const res = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${gatewayKey}`,
      },
      body: JSON.stringify({
        model: 'smart-combo',
        messages: [{ role: 'user', content: 'Non-stream combo test' }],
        stream: false,
      }),
    });
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.equal(body.choices[0].message.content, 'Zyrouter E2E Non-Stream OK!');
    console.log('  -> PASS: non-streaming combo request successfully routed');
  }

  // Scenario 10: Model Access Policy Restriction (Denied model)
  {
    console.log('[TEST 10] Testing policy enforcement on disallowed model...');
    // Create another alias that is NOT in the allowed_models list of the key
    await fetch(`${baseUrl}/api/model-aliases`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        alias: 'unauthorized-model',
        target: 'openai/mock-raw-gpt',
        isActive: 1,
      }),
    });

    const res = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${gatewayKey}`,
      },
      body: JSON.stringify({
        model: 'unauthorized-model',
        messages: [{ role: 'user', content: 'Should fail' }],
      }),
    });
    assert.equal(res.status, 403, 'Disallowed model request must return HTTP 403 Forbidden');
    console.log('  -> PASS: unauthorized model access blocked (HTTP 403)');
  }

  // Scenario 11: Security Summary & Hashed Keys at Rest
  {
    console.log('[TEST 11] Checking security summary & hash at rest...');
    const res = await fetch(`${baseUrl}/api/admin/security/summary`, {
      headers: { Cookie: cookieHeader },
    });
    assert.equal(res.status, 200);
    const summary = await res.json();
    assert.ok(summary.totalKeys >= 1, 'Summary must count total keys');
    assert.ok(summary.hashedKeys >= 1, 'Summary must record hashed keys');
    assert.equal(summary.migrationMarker, 'complete');
    console.log('  -> PASS: security summary verified (hashed at rest)');
  }

  // Scenario 12: Account Type Model Governance & Tier Enforcement
  {
    console.log('[TEST 12] Testing Account Type tier creation & model restriction...');
    // 1. Create custom account type
    const createTypeRes = await fetch(`${baseUrl}/api/admin/account-types`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        id: 'e2e-tier',
        name: 'E2E Tier',
        quotaMode: 'unlimited',
      }),
    });
    assert.equal(createTypeRes.status, 200);

    // 2. Allow only 'fast-llm' for this tier
    const setModelsRes = await fetch(`${baseUrl}/api/admin/account-types/e2e-tier/models`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({ aliases: ['fast-llm'] }),
    });
    assert.equal(setModelsRes.status, 200);

    // 3. Create API key assigned to 'e2e-tier'
    const keyRes = await fetch(`${baseUrl}/api/keys`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Cookie: cookieHeader },
      body: JSON.stringify({
        name: 'e2e-tier-key',
        accountTypeId: 'e2e-tier',
      }),
    });
    assert.equal(keyRes.status, 201);
    const tierKeyData = await keyRes.json();
    assert.equal(tierKeyData.accountTypeId, 'e2e-tier');

    // 4. Allowed model request must succeed
    const allowedChat = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${tierKeyData.key}`,
      },
      body: JSON.stringify({
        model: 'fast-llm',
        messages: [{ role: 'user', content: 'hello' }],
      }),
    });
    assert.equal(allowedChat.status, 200, 'Model allowed for account type must succeed');

    // 5. Published model not assigned to this tier must be blocked (HTTP 403)
    const blockedChat = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${tierKeyData.key}`,
      },
      body: JSON.stringify({
        model: 'smart-combo',
        messages: [{ role: 'user', content: 'hello' }],
      }),
    });
    assert.equal(blockedChat.status, 403, 'Model not in account type models must return 403 Forbidden');
    console.log('  -> PASS: tier-based model restriction strictly enforced (HTTP 403 on unassigned alias)');
  }

  // Scenario 13: Model Alias Test Endpoint
  {
    console.log('[TEST 13] Testing POST /api/model-aliases/{alias}/test...');
    const testRes = await fetch(`${baseUrl}/api/model-aliases/fast-llm/test`, {
      method: 'POST',
      headers: { Cookie: cookieHeader },
    });
    assert.equal(testRes.status, 200);
    const testData = await testRes.json();
    assert.equal(testData.status, 'ok');
    assert.equal(testData.resolved, true);
    console.log('  -> PASS: model alias test endpoint verified');
  }

  console.log('================================================================');
  console.log('🎉 ALL 13 E2E PROXY & GOVERNANCE INTEGRATION SCENARIOS PASSED! 🎉');
  console.log('================================================================');
} finally {
  // Graceful teardown
  console.log('[*] Tearing down test servers and cleaning up artifacts...');
  proxyProcess.kill('SIGTERM');
  mockServer.close();

  // Wait a moment for SQLite locks to release, then delete test sqlite files
  await new Promise((r) => setTimeout(r, 800));
  for (const suffix of ['', '-wal', '-shm']) {
    try {
      if (existsSync(dbPath + suffix)) {
        await rm(dbPath + suffix, { force: true });
      }
    } catch {}
  }
}

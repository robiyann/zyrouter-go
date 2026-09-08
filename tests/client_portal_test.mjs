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
const dbPath = path.join(__dirname, 'client_portal_test.sqlite');
const testPort = 21399;
const testHost = '127.0.0.1';
const adminPassword = 'client-portal-pass-2026';
const tgWebhookSecret = 'tg-secret-token-test-2026';

console.log('--- Starting Zyrouter Client Portal & Telegram Auth Test ---');

// 1. Clean previous test database
for (const suffix of ['', '-wal', '-shm']) {
  if (existsSync(dbPath + suffix)) {
    await rm(dbPath + suffix, { force: true });
  }
}

// 2. Ensure latest binary is built
const binPath = path.join(backendDir, 'zyrouter.exe');
console.log('[*] Building latest backend binary...');
const build = spawn('go', ['build', '-trimpath', '-o', 'zyrouter.exe', './cmd/zyrouter'], {
  cwd: backendDir,
  stdio: 'inherit',
  shell: true,
});
await new Promise((res, rej) => {
  build.on('close', (code) => (code === 0 ? res() : rej(new Error(`go build failed with code ${code}`))));
});

// 3. Spawn Zyrouter process
const proxyProcess = spawn(binPath, [], {
  cwd: backendDir,
  env: {
    ...process.env,
    HOST: testHost,
    PORT: String(testPort),
    DB_PATH: dbPath,
    INITIAL_PASSWORD: adminPassword,
    TELEGRAM_WEBHOOK_SECRET: tgWebhookSecret,
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

let teardownDone = false;
async function teardown() {
  if (teardownDone) return;
  teardownDone = true;
  proxyProcess.kill('SIGKILL');
  for (const suffix of ['', '-wal', '-shm']) {
    if (existsSync(dbPath + suffix)) {
      await rm(dbPath + suffix, { force: true }).catch(() => {});
    }
  }
}

process.on('exit', () => teardown());
process.on('SIGINT', () => { teardown(); process.exit(1); });
process.on('SIGTERM', () => { teardown(); process.exit(1); });

// Wait for server ready
const baseUrl = `http://${testHost}:${testPort}`;
let serverReady = false;
for (let i = 0; i < 40; i++) {
  try {
    const res = await fetch(`${baseUrl}/health`);
    if (res.ok) {
      serverReady = true;
      break;
    }
  } catch {
    await new Promise((r) => setTimeout(r, 200));
  }
}
assert.ok(serverReady, 'Zyrouter server failed to start');
console.log(`[✓] Zyrouter Server ready at ${baseUrl}`);

try {
  // TEST 1: Verify standalone zyrouter-client app files exist
  console.log('[TEST 1] Testing standalone zyrouter-client files & server API...');
  const clientDir = path.resolve(rootDir, '..', 'zyrouter-client');
  assert.ok(existsSync(path.join(clientDir, 'index.html')), 'zyrouter-client/index.html must exist');
  assert.ok(existsSync(path.join(clientDir, 'styles.css')), 'zyrouter-client/styles.css must exist');
  assert.ok(existsSync(path.join(clientDir, 'app.js')), 'zyrouter-client/app.js must exist');
  assert.ok(existsSync(path.join(clientDir, 'package.json')), 'zyrouter-client/package.json must exist');
  const helloRes = await fetch(`${baseUrl}/api/hello`);
  assert.equal(helloRes.status, 200);
  console.log('  -> PASS: standalone zyrouter-client directory and server API confirmed');

  // TEST 2: Telegram verification challenge start
  console.log('[TEST 2] Testing POST /api/user/verification/start...');
  const startRes = await fetch(`${baseUrl}/api/user/verification/start`, { method: 'POST' });
  assert.equal(startRes.status, 201, 'start verification must return 201 Created');
  const startData = await startRes.json();
  assert.ok(startData.challengeId, 'must return challengeId');
  assert.equal(startData.status, 'pending', 'initial status must be pending');
  const challengeId = startData.challengeId;
  const verificationCookie = (startRes.headers.get('set-cookie') || '').split(';')[0];
  console.log(`  -> PASS: challenge created (${challengeId})`);

  // TEST 3: Inspect challenge status before webhook
  console.log('[TEST 3] Testing GET /api/user/verification/{id} before verification...');
  const inspectRes = await fetch(`${baseUrl}/api/user/verification/${challengeId}`, { headers: { Cookie: verificationCookie } });
  assert.equal(inspectRes.status, 200);
  const inspectData = await inspectRes.json();
  assert.equal(inspectData.status, 'pending');
  assert.equal(inspectData.user, undefined, 'pending challenge must not return user');
  console.log('  -> PASS: challenge correctly reports pending state');

  // TEST 4: Telegram Webhook verification
  console.log('[TEST 4] Testing POST /api/telegram/webhook...');
  // Invalid secret check
  const badSecretRes = await fetch(`${baseUrl}/api/telegram/webhook`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Telegram-Bot-Api-Secret-Token': 'wrong-secret',
    },
    body: JSON.stringify({ message: { from: { id: 123 }, text: '/start ' + challengeId } }),
  });
  assert.equal(badSecretRes.status, 401, 'invalid webhook secret must be rejected');

  // Valid webhook call
  const webhookRes = await fetch(`${baseUrl}/api/telegram/webhook`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Telegram-Bot-Api-Secret-Token': tgWebhookSecret,
    },
    body: JSON.stringify({
      message: {
        from: {
          id: 777888999,
          username: 'cybergod',
          first_name: 'Neo',
          last_name: 'Anderson',
        },
        text: `/start ${challengeId}`,
      },
    }),
  });
  assert.equal(webhookRes.status, 200);
  const webhookData = await webhookRes.json();
  assert.equal(webhookData.status, 'verified', 'webhook response must be verified');
  console.log('  -> PASS: telegram webhook verified identity (cybergod / 777888999)');

  // TEST 5: Poll challenge status after verification -> Obtain user_session cookie
  console.log('[TEST 5] Testing challenge polling & HttpOnly cookie issuance...');
  const pollRes = await fetch(`${baseUrl}/api/user/verification/${challengeId}`, { headers: { Cookie: verificationCookie } });
  assert.equal(pollRes.status, 200);
  const cookieHeader = pollRes.headers.get('set-cookie') || '';
  assert.match(cookieHeader, /user_session=/, 'set-cookie must issue user_session cookie');
  assert.match(cookieHeader, /HttpOnly/i, 'session cookie must be HttpOnly');

  const pollData = await pollRes.json();
  assert.equal(pollData.status, 'verified');
  assert.ok(pollData.user, 'must return user object');
  assert.equal(pollData.user.telegramUsername, 'cybergod');
  assert.equal(pollData.sessionToken, undefined, 'session token must not be exposed to browser JavaScript');

  const sessionCookie = cookieHeader.split(';')[0];
  console.log(`  -> PASS: user verified, session cookie received (${sessionCookie.slice(0, 24)}...)`);

  // Helper for authenticated user requests
  async function userFetch(endpoint, opts = {}) {
    return fetch(`${baseUrl}${endpoint}`, {
      ...opts,
      headers: {
        ...opts.headers,
        Cookie: sessionCookie,
      },
    });
  }

  // TEST 6: User Profile & Governance Endpoints
  console.log('[TEST 6] Testing GET /api/user/profile...');
  const profileRes = await userFetch('/api/user/profile');
  assert.equal(profileRes.status, 200);
  const profileData = await profileRes.json();
  assert.equal(profileData.user.telegramUsername, 'cybergod');
  assert.ok(profileData.accountType, 'must return accountType');
  assert.ok(Array.isArray(profileData.allowedAliases), 'must return allowedAliases array');
  console.log(`  -> PASS: profile retrieved for tier ${profileData.accountType.name}`);

  // TEST 7: User Usage Aggregates
  console.log('[TEST 7] Testing GET /api/user/usage...');
  const usageRes = await userFetch('/api/user/usage');
  assert.equal(usageRes.status, 200);
  const usageData = await usageRes.json();
  assert.equal(typeof usageData.totalRequests, 'number');
  assert.equal(typeof usageData.totalTokens, 'number');
  assert.equal(typeof usageData.totalCost, 'number');
  console.log('  -> PASS: usage aggregates retrieved');

  // TEST 7b: Private request log history and SSE stream are user-session scoped
  console.log('[TEST 7b] Testing private client logs history and SSE stream...');
  const logsRes = await userFetch('/api/user/logs?limit=10&offset=0');
  assert.equal(logsRes.status, 200);
  const logsData = await logsRes.json();
  assert.ok(Array.isArray(logsData.items));
  const logAbort = new AbortController();
  const streamRes = await userFetch('/api/user/logs/stream', { signal: logAbort.signal });
  assert.equal(streamRes.status, 200);
  assert.match(streamRes.headers.get('content-type') || '', /text\/event-stream/);
  const firstChunk = await streamRes.body.getReader().read();
  assert.match(new TextDecoder().decode(firstChunk.value), /event: snapshot/);
  logAbort.abort();
  const adminUserLogRes = await fetch(`${baseUrl}/api/user/logs`, { headers: { Cookie: 'auth_token=admin-session' } });
  assert.equal(adminUserLogRes.status, 401, 'admin session must not become a user session');
  console.log('  -> PASS: client logs are private and stream starts with a snapshot');

  // TEST 8: Feature Toggles
  console.log('[TEST 8] Testing GET & PUT /api/user/features...');
  const featRes = await userFetch('/api/user/features');
  assert.equal(featRes.status, 200);

  // If tier allows features, test toggle
  if (profileData.accountType.allowRTK) {
    const putFeatRes = await userFetch('/api/user/features', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rtkEnabled: true, cavemanEnabled: false, ponytailEnabled: false }),
    });
    assert.equal(putFeatRes.status, 200);
    const updatedFeat = await putFeatRes.json();
    assert.equal(updatedFeat.rtkEnabled, true);
  }
  console.log('  -> PASS: user features inspected and enforced');

  // TEST 9: Single Active API Key Invariant
  console.log('[TEST 9] Testing Single Active API Key (1 User = 1 Key)...');
  const initialKeyRes = await userFetch('/api/user/key');
  assert.equal(initialKeyRes.status, 200);
  const initialKeyData = await initialKeyRes.json();
  assert.equal(initialKeyData.key, null, 'initially user must have no active key');

  // Generate First Key
  const genKeyRes = await userFetch('/api/user/key', { method: 'POST' });
  assert.equal(genKeyRes.status, 201, 'key generation must return 201 Created');
  const genKeyData = await genKeyRes.json();
  assert.match(genKeyData.key, /^zy_/, 'generated key must start with zy_');
  const firstRawKey = genKeyData.key;

  // Duplicate Generate must be rejected with 409 Conflict
  const dupKeyRes = await userFetch('/api/user/key', { method: 'POST' });
  assert.equal(dupKeyRes.status, 409, 'duplicate key generation must return 409 Conflict');
  console.log('  -> PASS: first key created, duplicate generate blocked (409 Conflict)');

  // Rotate Key
  console.log('[TEST 10] Testing POST /api/user/key/rotate...');
  const rotateRes = await userFetch('/api/user/key/rotate', { method: 'POST' });
  assert.equal(rotateRes.status, 201, 'rotate must return 201 Created');
  const rotateData = await rotateRes.json();
  assert.match(rotateData.key, /^zy_/, 'rotated key must start with zy_');
  assert.notEqual(rotateData.key, firstRawKey, 'rotated key must be new token');
  console.log('  -> PASS: key successfully rotated');

  // Revoke Key
  console.log('[TEST 11] Testing DELETE /api/user/key...');
  const revokeRes = await userFetch('/api/user/key', { method: 'DELETE' });
  assert.equal(revokeRes.status, 200);
  const postRevokeRes = await userFetch('/api/user/key');
  const postRevokeData = await postRevokeRes.json();
  assert.equal(postRevokeData.key, null, 'after revoke, active key must be null');
  console.log('  -> PASS: key revoked');

  // TEST 12: Logout
  console.log('[TEST 12] Testing POST /api/user/logout...');
  const logoutRes = await userFetch('/api/user/logout', { method: 'POST' });
  assert.equal(logoutRes.status, 200);
  const logoutCookie = logoutRes.headers.get('set-cookie') || '';
  assert.match(logoutCookie, /user_session=;/, 'logout must clear session cookie');

  // Ensure unauthenticated now
  const postLogoutRes = await userFetch('/api/user/profile');
  assert.equal(postLogoutRes.status, 401, 'post-logout profile must return 401 Unauthorized');
  console.log('  -> PASS: logout successfully revoked session and cleared cookie');

  // TEST 13: Machine Client Provisioning by Admin
  console.log('[TEST 13] Testing Admin Machine Client Provisioning (/api/admin/clients)...');
  const adminLoginRes = await fetch(`${baseUrl}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: adminPassword }),
  });
  assert.equal(adminLoginRes.status, 200);
  const adminCookie = adminLoginRes.headers.get('set-cookie')?.split(';')[0] || '';

  const createPolicyRes = await fetch(`${baseUrl}/api/admin/client-policies`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: adminCookie },
    body: JSON.stringify({
      name: 'Enterprise Client Policy',
      allowedModels: ['unified-chat'],
    }),
  });
  assert.equal(createPolicyRes.status, 201);
  const createdPolicy = await createPolicyRes.json();

  const createClientRes = await fetch(`${baseUrl}/api/admin/clients`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: adminCookie },
    body: JSON.stringify({
      name: 'Acme Enterprise',
      email: 'dev@acme.corp',
      policyId: createdPolicy.id,
    }),
  });
  assert.equal(createClientRes.status, 201);
  const createdClientData = await createClientRes.json();
  assert.ok(createdClientData.accessToken, 'must return client accessToken');
  const cltToken = createdClientData.accessToken;
  console.log('  -> PASS: admin provisioned machine client & issued clt_ token');

  // TEST 14: Machine Client API Access (/api/client/*)
  console.log('[TEST 14] Testing Machine Client Flow with clt_ token...');
  const cltHeaders = { Authorization: `Bearer ${cltToken}` };
  const cltProfileRes = await fetch(`${baseUrl}/api/client/profile`, { headers: cltHeaders });
  assert.equal(cltProfileRes.status, 200);
  const cltProfile = await cltProfileRes.json();
  assert.equal(cltProfile.name, 'Acme Enterprise');

  const cltPolicyRes = await fetch(`${baseUrl}/api/client/policy`, { headers: cltHeaders });
  assert.equal(cltPolicyRes.status, 200);
  const cltPolicy = await cltPolicyRes.json();
  assert.deepEqual(cltPolicy.allowedModels, ['unified-chat']);

  // Create Machine API Key
  const cltCreateKeyRes = await fetch(`${baseUrl}/api/client/keys`, {
    method: 'POST',
    headers: { ...cltHeaders, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: 'Acme Production Key' }),
  });
  assert.equal(cltCreateKeyRes.status, 201);
  const cltKeyData = await cltCreateKeyRes.json();
  assert.match(cltKeyData.key, /^zy_/);

  // List Machine Keys
  const cltListKeysRes = await fetch(`${baseUrl}/api/client/keys`, { headers: cltHeaders });
  assert.equal(cltListKeysRes.status, 200);
  const cltKeysList = await cltListKeysRes.json();
  assert.equal(cltKeysList.keys.length, 1);

  // Revoke Machine Key
  const cltRevokeRes = await fetch(`${baseUrl}/api/client/keys/${cltKeyData.id}`, {
    method: 'DELETE',
    headers: cltHeaders,
  });
  assert.equal(cltRevokeRes.status, 200);
  console.log('  -> PASS: machine client authenticated, generated key, and revoked key');

  console.log('================================================================');
  console.log('🎉 ALL 14 CLIENT PORTAL & MULTI-AUTH TESTS PASSED! 🎉');
  console.log('================================================================');

} finally {
  await teardown();
}

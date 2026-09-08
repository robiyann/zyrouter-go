/**
 * Zyrouter Client Dashboard (HeroUI Design System)
 * High-security client interface for Telegram Verified Users and Machine Clients.
 * Strict zero data leakage: No provider credentials, no raw models, no internal targets.
 */

(() => {
  'use strict';

  // --- State ---
  let authType = 'telegram'; // 'telegram' | 'machine'
  let sessionToken = localStorage.getItem('zy_client_session') || '';
  let machineToken = localStorage.getItem('zy_machine_token') || '';
  let activeUser = null;
  let activeAccountType = null;
  let allowedAliases = [];
  let activeKey = null;
  let activeFeatures = { rtkEnabled: false, cavemanEnabled: false, ponytailEnabled: false };
  let challengeInterval = null;
  let challengeTimerInterval = null;
  let playgroundAbortCtrl = null;
  let currentSnippetType = 'curl';

  // --- Utilities ---
  function escapeHtml(str) {
    if (str === null || str === undefined) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  async function copyText(value) {
    if (!value) return false;
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(value);
        return true;
      }
    } catch (_) {}
    const ta = document.createElement('textarea');
    ta.value = value;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    ta.style.top = '-9999px';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    let ok = false;
    try {
      ok = document.execCommand('copy');
    } catch (_) {
      ok = false;
    }
    ta.remove();
    return ok;
  }

  function showToast(message, type = 'default') {
    const container = document.getElementById('toast-container');
    if (!container) return;
    const toast = document.createElement('div');
    toast.className = `hero-toast ${type}`;
    const iconName = type === 'success' ? 'check_circle' : type === 'error' ? 'error' : 'info';
    toast.innerHTML = `
      <span class="material-symbols-outlined" style="font-size: 18px;">${iconName}</span>
      <span>${escapeHtml(message)}</span>
    `;
    container.appendChild(toast);
    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateY(10px)';
      toast.style.transition = 'all 0.2s ease';
      setTimeout(() => toast.remove(), 200);
    }, 3500);
  }

  function openModal(id) {
    const modal = document.getElementById(id);
    if (modal) modal.classList.add('open');
  }

  function closeModal(id) {
    const modal = document.getElementById(id);
    if (modal) modal.classList.remove('open');
  }

  // --- API Client ---
  async function apiFetch(endpoint, options = {}) {
    const headers = options.headers || {};
    if (authType === 'machine' && machineToken) {
      headers['Authorization'] = `Bearer ${machineToken}`;
    } else if (sessionToken) {
      headers['Authorization'] = `Bearer ${sessionToken}`;
    }

    const config = {
      ...options,
      headers,
      credentials: 'include', // Ensures HttpOnly cookie user_session is automatically forwarded
    };

    const res = await fetch(endpoint, config);
    if (res.status === 401) {
      // Session expired or unauthorized
      if (document.getElementById('dashboard-container').style.display !== 'none') {
        showToast('Session expired. Please log in again.', 'error');
        handleLogout();
      }
      throw new Error('Unauthorized');
    }
    return res;
  }

  // --- Telegram Verification Flow ---
  async function startTelegramVerification() {
    const btnStart = document.getElementById('btn-start-verification');
    btnStart.disabled = true;
    btnStart.innerHTML = `<span class="material-symbols-outlined" style="animation: spin 1s infinite linear;">refresh</span> Starting...`;

    try {
      const res = await fetch('/api/user/verification/start', { method: 'POST' });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Failed to initiate verification challenge`);
      }
      const data = await res.json();
      const challengeId = data.challengeId;
      const deepLink = data.telegramDeepLink || `https://t.me/zyrouter_bot?start=${challengeId}`;

      // Update UI for active challenge
      document.getElementById('tg-start-box').style.display = 'none';
      document.getElementById('tg-challenge-box').style.display = 'flex';
      document.getElementById('challenge-id-label').textContent = challengeId;
      
      const btnDeepLink = document.getElementById('btn-tg-deep-link');
      btnDeepLink.href = deepLink;

      // Start countdown timer
      let timeLeftSec = 300; // 5 minutes
      const timerEl = document.getElementById('challenge-timer');
      if (challengeTimerInterval) clearInterval(challengeTimerInterval);
      challengeTimerInterval = setInterval(() => {
        timeLeftSec--;
        if (timeLeftSec <= 0) {
          cancelTelegramChallenge();
          showToast('Verification challenge expired. Please start again.', 'error');
          return;
        }
        const m = Math.floor(timeLeftSec / 60);
        const s = timeLeftSec % 60;
        timerEl.textContent = `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
      }, 1000);

      // Start polling
      if (challengeInterval) clearInterval(challengeInterval);
      challengeInterval = setInterval(async () => {
        try {
          const pollRes = await fetch(`/api/user/verification/${encodeURIComponent(challengeId)}`);
          if (pollRes.ok) {
            const pollData = await pollRes.json();
            if (pollData.status === 'verified' && pollData.user) {
              clearInterval(challengeInterval);
              clearInterval(challengeTimerInterval);
              challengeInterval = null;
              challengeTimerInterval = null;

              if (pollData.sessionToken) {
                sessionToken = pollData.sessionToken;
                localStorage.setItem('zy_client_session', sessionToken);
              }
              authType = 'telegram';
              showToast('Telegram verification successful!', 'success');
              bootstrapDashboard();
            }
          }
        } catch (_) {}
      }, 2000);

    } catch (err) {
      showToast(err.message || 'Failed to start Telegram verification', 'error');
      btnStart.disabled = false;
      btnStart.innerHTML = `<span class="material-symbols-outlined">verified_user</span> Start Telegram Verification`;
    }
  }

  function cancelTelegramChallenge() {
    if (challengeInterval) clearInterval(challengeInterval);
    if (challengeTimerInterval) clearInterval(challengeTimerInterval);
    challengeInterval = null;
    challengeTimerInterval = null;

    document.getElementById('tg-challenge-box').style.display = 'none';
    const startBox = document.getElementById('tg-start-box');
    startBox.style.display = 'block';
    const btnStart = document.getElementById('btn-start-verification');
    btnStart.disabled = false;
    btnStart.innerHTML = `<span class="material-symbols-outlined">verified_user</span> Start Telegram Verification`;
  }

  // --- Machine Client Login Flow ---
  async function loginMachineClient() {
    const input = document.getElementById('input-client-token');
    const token = (input.value || '').trim();
    if (!token) {
      showToast('Please enter your client access token', 'error');
      return;
    }
    const btn = document.getElementById('btn-login-clt');
    btn.disabled = true;

    try {
      const res = await fetch('/api/client/profile', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!res.ok) {
        throw new Error('Invalid or inactive machine client token');
      }
      machineToken = token;
      localStorage.setItem('zy_machine_token', machineToken);
      authType = 'machine';
      showToast('Connected as Machine Client', 'success');
      bootstrapDashboard();
    } catch (err) {
      showToast(err.message || 'Login failed', 'error');
    } finally {
      btn.disabled = false;
    }
  }

  // --- Dashboard Bootstrap ---
  async function bootstrapDashboard() {
    try {
      if (authType === 'telegram') {
        await loadTelegramUserData();
      } else {
        await loadMachineClientData();
      }

      // Show Dashboard Shell
      document.getElementById('auth-container').style.display = 'none';
      document.getElementById('dashboard-container').style.display = 'flex';
      document.getElementById('nav-user-area').style.display = 'flex';

      // Load Quickstart and active tab view
      renderQuickstartCode();
    } catch (err) {
      console.error('Failed to bootstrap dashboard:', err);
      // If unauthorized, return to login
      document.getElementById('auth-container').style.display = 'block';
      document.getElementById('dashboard-container').style.display = 'none';
      document.getElementById('nav-user-area').style.display = 'none';
    }
  }

  async function loadTelegramUserData() {
    // 1. Profile & Account Tier
    const profileRes = await apiFetch('/api/user/profile');
    if (!profileRes.ok) throw new Error('Failed to load profile');
    const profileData = await profileRes.json();
    activeUser = profileData.user;
    activeAccountType = profileData.accountType;
    allowedAliases = profileData.allowedAliases || [];

    // Render User Badge in Header
    const initial = (activeUser.displayName || activeUser.telegramUsername || 'U').charAt(0).toUpperCase();
    document.getElementById('user-avatar-initials').textContent = initial;
    document.getElementById('user-display-name').textContent = activeUser.displayName || `@${activeUser.telegramUsername || 'user'}`;
    document.getElementById('user-tier-name').textContent = `TIER: ${(activeAccountType?.name || 'Standard').toUpperCase()}`;

    // Render Tier Details Card
    document.getElementById('tier-name-label').textContent = activeAccountType?.name || 'Default';
    document.getElementById('tier-desc-label').textContent = activeAccountType?.description || 'Enforced alias-only routing';
    document.getElementById('tier-models-count').textContent = `${allowedAliases.length} Published Aliases`;

    const saverBadgesBox = document.getElementById('tier-saver-badges');
    saverBadgesBox.innerHTML = `
      <span class="hero-chip ${activeAccountType?.allowRTK ? 'hero-chip-emerald' : 'hero-chip-zinc'}">RTK</span>
      <span class="hero-chip ${activeAccountType?.allowCaveman ? 'hero-chip-emerald' : 'hero-chip-zinc'}">Caveman</span>
      <span class="hero-chip ${activeAccountType?.allowPonytail ? 'hero-chip-emerald' : 'hero-chip-zinc'}">Ponytail</span>
    `;

    // 2. Usage Metrics
    await refreshUsageMetrics();

    // 3. Active Gateway Key
    await refreshKeyStatus();

    // 4. Feature Settings
    await refreshFeatureSettings();

    // 5. Render Allowed Models Grid
    renderAllowedModels();
  }

  async function loadMachineClientData() {
    // Machine Client Profile & Policy
    const profRes = await apiFetch('/api/client/profile');
    if (!profRes.ok) throw new Error('Failed to load machine profile');
    const client = await profRes.json();

    const policyRes = await apiFetch('/api/client/policy');
    let policy = {};
    if (policyRes.ok) {
      policy = await policyRes.json();
    }
    allowedAliases = policy.allowedModels || [];

    // Render Machine Identity
    document.getElementById('user-avatar-initials').textContent = 'M';
    document.getElementById('user-display-name').textContent = client.name || client.id;
    document.getElementById('user-tier-name').textContent = 'MACHINE CLIENT';

    document.getElementById('tier-name-label').textContent = policy.name || 'Machine Client Policy';
    document.getElementById('tier-desc-label').textContent = 'Server-side key management';
    document.getElementById('tier-models-count').textContent = `${allowedAliases.length} Aliases`;

    // Usage
    const usageRes = await apiFetch('/api/client/usage');
    if (usageRes.ok) {
      const usage = await usageRes.json();
      renderUsageValues(usage);
    }

    // Machine Keys
    await refreshMachineKeys();

    // Render Allowed Models
    renderAllowedModels();
  }

  async function refreshUsageMetrics() {
    try {
      const res = await apiFetch('/api/user/usage');
      if (res.ok) {
        const usage = await res.json();
        renderUsageValues(usage);
      }
    } catch (_) {}
  }

  function renderUsageValues(usage = {}) {
    const reqs = Number(usage.totalRequests || 0);
    const pTokens = Number(usage.promptTokens || 0);
    const cTokens = Number(usage.completionTokens || 0);
    const tTokens = Number(usage.totalTokens || (pTokens + cTokens));
    const cost = Number(usage.totalCost || 0);

    document.getElementById('metric-requests').textContent = reqs.toLocaleString();
    document.getElementById('metric-total-tokens').textContent = tTokens.toLocaleString();
    document.getElementById('metric-tokens-breakdown').textContent = `${pTokens.toLocaleString()} / ${cTokens.toLocaleString()}`;
    document.getElementById('metric-cost').textContent = `$${cost.toFixed(4)}`;
  }

  // --- Key Management ---
  async function refreshKeyStatus() {
    if (authType === 'machine') {
      return refreshMachineKeys();
    }
    try {
      const res = await apiFetch('/api/user/key');
      if (!res.ok) throw new Error('Failed to fetch user key');
      const data = await res.json();
      activeKey = data.key || null;

      const detailsBox = document.getElementById('key-active-details');
      const emptyBox = document.getElementById('key-empty-details');
      const chipBox = document.getElementById('key-status-chip-box');

      if (activeKey && activeKey.isActive === 1) {
        detailsBox.style.display = 'flex';
        emptyBox.style.display = 'none';
        chipBox.innerHTML = `<span class="hero-chip hero-chip-emerald"><span class="chip-dot"></span> 1 ACTIVE KEY</span>`;
        document.getElementById('active-key-prefix').textContent = `${activeKey.keyPrefix || 'zy_xxxxxxxx'}...`;
        document.getElementById('active-key-id').textContent = activeKey.id || '--';
        document.getElementById('active-key-created').textContent = activeKey.createdAt ? new Date(activeKey.createdAt).toLocaleString('id-ID') : '--';
      } else {
        detailsBox.style.display = 'none';
        emptyBox.style.display = 'flex';
        chipBox.innerHTML = `<span class="hero-chip hero-chip-zinc">NO KEY</span>`;
      }
    } catch (err) {
      console.error(err);
    }
  }

  async function refreshMachineKeys() {
    try {
      const res = await apiFetch('/api/client/keys');
      if (!res.ok) return;
      const data = await res.json();
      const keys = data.keys || [];
      const firstKey = keys.find(k => k.isActive === 1);

      const detailsBox = document.getElementById('key-active-details');
      const emptyBox = document.getElementById('key-empty-details');
      const chipBox = document.getElementById('key-status-chip-box');

      if (firstKey) {
        activeKey = firstKey;
        detailsBox.style.display = 'flex';
        emptyBox.style.display = 'none';
        chipBox.innerHTML = `<span class="hero-chip hero-chip-emerald"><span class="chip-dot"></span> ACTIVE (${keys.length} KEYS)</span>`;
        document.getElementById('active-key-prefix').textContent = `${firstKey.keyPrefix || 'zy_xxxxxxxx'}...`;
        document.getElementById('active-key-id').textContent = firstKey.id || '--';
        document.getElementById('active-key-created').textContent = firstKey.createdAt ? new Date(firstKey.createdAt).toLocaleString('id-ID') : '--';
      } else {
        detailsBox.style.display = 'none';
        emptyBox.style.display = 'flex';
        chipBox.innerHTML = `<span class="hero-chip hero-chip-zinc">NO KEY</span>`;
      }
    } catch (_) {}
  }

  async function handleGenerateKey() {
    const btn = document.getElementById('btn-generate-key');
    btn.disabled = true;
    try {
      const endpoint = authType === 'machine' ? '/api/client/keys' : '/api/user/key';
      const res = await apiFetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'Client Gateway Key' })
      });

      if (res.status === 409) {
        showToast('You already have an active API key. Please use Rotate Key instead.', 'error');
        await refreshKeyStatus();
        return;
      }
      if (!res.ok) {
        const errJson = await res.json().catch(() => ({}));
        throw new Error(errJson.error || `HTTP ${res.status}`);
      }

      const data = await res.json();
      const rawKey = data.key;
      // Show One-Time Reveal Modal
      document.getElementById('revealed-secret-text').textContent = rawKey;
      openModal('modal-reveal-secret');
      showToast('Gateway API Key created successfully!', 'success');
      await refreshKeyStatus();
      renderQuickstartCode(rawKey);
    } catch (err) {
      showToast(err.message || 'Failed to generate key', 'error');
    } finally {
      btn.disabled = false;
    }
  }

  async function handleRotateKey() {
    const btn = document.getElementById('btn-confirm-rotate');
    btn.disabled = true;
    try {
      const res = await apiFetch('/api/user/key/rotate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });

      if (!res.ok) {
        const errJson = await res.json().catch(() => ({}));
        throw new Error(errJson.error || `HTTP ${res.status}`);
      }

      const data = await res.json();
      const rawKey = data.key;
      closeModal('modal-confirm-rotate');
      document.getElementById('revealed-secret-text').textContent = rawKey;
      openModal('modal-reveal-secret');
      showToast('API Key rotated successfully!', 'success');
      await refreshKeyStatus();
      renderQuickstartCode(rawKey);
    } catch (err) {
      showToast(err.message || 'Failed to rotate key', 'error');
    } finally {
      btn.disabled = false;
    }
  }

  async function handleRevokeKey() {
    const btn = document.getElementById('btn-confirm-revoke');
    btn.disabled = true;
    try {
      const endpoint = authType === 'machine' && activeKey ? `/api/client/keys/${encodeURIComponent(activeKey.id)}` : '/api/user/key';
      const res = await apiFetch(endpoint, { method: 'DELETE' });

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Revoke failed`);
      }

      closeModal('modal-confirm-revoke');
      showToast('API Key revoked successfully', 'success');
      await refreshKeyStatus();
      renderQuickstartCode();
    } catch (err) {
      showToast(err.message || 'Failed to revoke key', 'error');
    } finally {
      btn.disabled = false;
    }
  }

  // --- Feature Toggles ---
  async function refreshFeatureSettings() {
    if (authType !== 'telegram') return;
    try {
      const res = await apiFetch('/api/user/features');
      if (res.ok) {
        const settings = await res.json();
        activeFeatures.rtkEnabled = !!settings.rtkEnabled;
        activeFeatures.cavemanEnabled = !!settings.cavemanEnabled;
        activeFeatures.ponytailEnabled = !!settings.ponytailEnabled;
      }
      updateFeatureTogglesUI();
    } catch (_) {}
  }

  function updateFeatureTogglesUI() {
    const rtkToggle = document.getElementById('toggle-feature-rtk');
    const cavemanToggle = document.getElementById('toggle-feature-caveman');
    const ponytailToggle = document.getElementById('toggle-feature-ponytail');

    // Check tier permissions
    if (activeAccountType) {
      rtkToggle.classList.toggle('disabled', !activeAccountType.allowRTK);
      cavemanToggle.classList.toggle('disabled', !activeAccountType.allowCaveman);
      ponytailToggle.classList.toggle('disabled', !activeAccountType.allowPonytail);
    }

    rtkToggle.classList.toggle('active', !!activeFeatures.rtkEnabled);
    cavemanToggle.classList.toggle('active', !!activeFeatures.cavemanEnabled);
    ponytailToggle.classList.toggle('active', !!activeFeatures.ponytailEnabled);
  }

  async function toggleFeature(featureName) {
    if (authType !== 'telegram') return;
    
    // Check account type policy guard
    if (featureName === 'rtkEnabled' && !activeAccountType?.allowRTK) {
      showToast('RTK compression is not allowed for your account tier.', 'error');
      return;
    }
    if (featureName === 'cavemanEnabled' && !activeAccountType?.allowCaveman) {
      showToast('Caveman mode is not allowed for your account tier.', 'error');
      return;
    }
    if (featureName === 'ponytailEnabled' && !activeAccountType?.allowPonytail) {
      showToast('Ponytail style is not allowed for your account tier.', 'error');
      return;
    }

    activeFeatures[featureName] = !activeFeatures[featureName];
    updateFeatureTogglesUI();

    try {
      const res = await apiFetch('/api/user/features', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(activeFeatures)
      });
      if (!res.ok) {
        throw new Error('Failed to update feature settings');
      }
      showToast('Feature preferences saved', 'success');
    } catch (err) {
      activeFeatures[featureName] = !activeFeatures[featureName];
      updateFeatureTogglesUI();
      showToast(err.message, 'error');
    }
  }

  // --- Allowed Models Grid ---
  function renderAllowedModels() {
    const grid = document.getElementById('models-list-grid');
    const select = document.getElementById('playground-model-select');
    if (!grid || !select) return;

    if (!allowedAliases || allowedAliases.length === 0) {
      grid.innerHTML = `
        <div style="grid-column: 1 / -1; text-align: center; color: var(--text-muted); padding: 36px;">
          No model aliases are assigned to your tier yet. Contact your router administrator.
        </div>
      `;
      select.innerHTML = `<option value="">No authorized models</option>`;
      return;
    }

    grid.innerHTML = allowedAliases.map((alias) => `
      <div style="background: rgba(18, 18, 20, 0.7); border: 1px solid var(--border-subtle); border-radius: var(--radius-lg); padding: 18px; display: flex; flex-direction: column; justify-content: space-between; gap: 14px; transition: border-color 0.2s ease;">
        <div>
          <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px;">
            <code style="font-size: 13.5px; font-weight: 700; color: var(--text-primary); font-family: var(--font-mono);">${escapeHtml(alias)}</code>
            <span class="hero-chip hero-chip-cyan" style="font-size: 9.5px;">PUBLIC ALIAS</span>
          </div>
          <p style="font-size: 12px; color: var(--text-muted); margin-top: 6px;">
            Authorized for chat completions through /v1/chat/completions.
          </p>
        </div>
        <div style="display: flex; justify-content: flex-end;">
          <button class="hero-btn hero-btn-flat hero-btn-sm btn-try-model" data-alias="${escapeHtml(alias)}">
            <span class="material-symbols-outlined" style="font-size: 14px;">play_arrow</span>
            Test in Playground
          </button>
        </div>
      </div>
    `).join('');

    select.innerHTML = allowedAliases.map(alias => `<option value="${escapeHtml(alias)}">${escapeHtml(alias)}</option>`).join('');

    // Attach click handlers to "Test in Playground" buttons
    grid.querySelectorAll('.btn-try-model').forEach(btn => {
      btn.addEventListener('click', (e) => {
        const alias = e.currentTarget.dataset.alias;
        if (alias) {
          select.value = alias;
          switchTab('playground');
        }
      });
    });
  }

  // --- Quickstart Code Generator ---
  function renderQuickstartCode(revealedKey = null) {
    const box = document.getElementById('code-snippet-box');
    if (!box) return;

    const host = window.location.origin;
    const model = allowedAliases[0] || 'unified-chat';
    const keyVal = revealedKey || (activeKey?.keyPrefix ? `${activeKey.keyPrefix}xxxxxxxx` : '<YOUR_GATEWAY_API_KEY>');

    let code = '';
    if (currentSnippetType === 'curl') {
      code = `curl -X POST "${host}/v1/chat/completions" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${keyVal}" \\
  -d '{
    "model": "${model}",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello Zyrouter!"}
    ],
    "stream": true
  }'`;
    } else if (currentSnippetType === 'python') {
      code = `from openai import OpenAI

client = OpenAI(
    base_url="${host}/v1",
    api_key="${keyVal}",
)

response = client.chat.completions.create(
    model="${model}",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello Zyrouter!"}
    ],
    stream=True,
)

for chunk in response:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()`;
    } else if (currentSnippetType === 'nodejs') {
      code = `import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "${host}/v1",
  apiKey: "${keyVal}",
});

async function main() {
  const stream = await openai.chat.completions.create({
    model: "${model}",
    messages: [
      { role: "system", content: "You are a helpful assistant." },
      { role: "user", content: "Hello Zyrouter!" }
    ],
    stream: true,
  });

  for await (const chunk of stream) {
    process.stdout.write(chunk.choices[0]?.delta?.content || "");
  }
  console.log();
}

main();`;
    }

    box.textContent = code;
  }

  // --- Chat Playground ---
  async function handleSendChat() {
    const input = document.getElementById('playground-input');
    const select = document.getElementById('playground-model-select');
    const messagesBox = document.getElementById('playground-messages');
    const btnSend = document.getElementById('btn-send-chat');
    const btnStop = document.getElementById('btn-stop-chat');

    const promptText = (input.value || '').trim();
    if (!promptText) return;

    const selectedModel = select.value;
    if (!selectedModel) {
      showToast('Please select an authorized model alias first', 'error');
      return;
    }

    input.value = '';
    btnSend.style.display = 'none';
    btnStop.style.display = 'inline-flex';

    // Append User Bubble
    const userMsgEl = document.createElement('div');
    userMsgEl.className = 'chat-msg user';
    userMsgEl.innerHTML = `
      <div class="chat-bubble">${escapeHtml(promptText)}</div>
      <span class="chat-meta">You</span>
    `;
    messagesBox.appendChild(userMsgEl);

    // Append Assistant Bubble placeholder
    const assistantMsgEl = document.createElement('div');
    assistantMsgEl.className = 'chat-msg assistant';
    assistantMsgEl.innerHTML = `
      <div class="chat-bubble" id="current-stream-bubble">
        <span style="display:inline-block; animation: pulseGlow 1.2s infinite ease;">● Thinking...</span>
      </div>
      <span class="chat-meta">${escapeHtml(selectedModel)}</span>
    `;
    messagesBox.appendChild(assistantMsgEl);
    messagesBox.scrollTop = messagesBox.scrollHeight;

    const streamBubble = assistantMsgEl.querySelector('#current-stream-bubble');
    playgroundAbortCtrl = new AbortController();

    try {
      const headers = { 'Content-Type': 'application/json' };
      if (activeKey?.keyPrefix) {
        // Notice: In client session, user_session cookie or bearer token is passed
        headers['Authorization'] = `Bearer ${sessionToken || machineToken || activeKey.keyPrefix}`;
      }

      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers,
        credentials: 'include',
        signal: playgroundAbortCtrl.signal,
        body: JSON.stringify({
          model: selectedModel,
          messages: [{ role: 'user', content: promptText }],
          stream: true
        })
      });

      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error?.message || errData.error || `HTTP ${res.status}`);
      }

      streamBubble.textContent = '';
      const reader = res.body.getReader();
      const decoder = new TextDecoder('utf-8');
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop(); // Keep partial line in buffer

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || !trimmed.startsWith('data:')) continue;
          const dataStr = trimmed.slice(5).trim();
          if (dataStr === '[DONE]') continue;

          try {
            const parsed = JSON.parse(dataStr);
            const token = parsed.choices?.[0]?.delta?.content || '';
            if (token) {
              streamBubble.textContent += token;
              messagesBox.scrollTop = messagesBox.scrollHeight;
            }
          } catch (_) {}
        }
      }

      // Refresh usage stats after chat
      refreshUsageMetrics();

    } catch (err) {
      if (err.name === 'AbortError') {
        streamBubble.textContent += ' [Stopped]';
      } else {
        streamBubble.innerHTML = `<span style="color: #fb7185;">Error: ${escapeHtml(err.message)}</span>`;
      }
    } finally {
      playgroundAbortCtrl = null;
      btnSend.style.display = 'inline-flex';
      btnStop.style.display = 'none';
      messagesBox.scrollTop = messagesBox.scrollHeight;
    }
  }

  function handleStopChat() {
    if (playgroundAbortCtrl) {
      playgroundAbortCtrl.abort();
    }
  }

  // --- Logout Handler ---
  async function handleLogout() {
    try {
      if (authType === 'telegram') {
        await fetch('/api/user/logout', { method: 'POST', credentials: 'include' });
      }
    } catch (_) {}

    sessionToken = '';
    machineToken = '';
    localStorage.removeItem('zy_client_session');
    localStorage.removeItem('zy_machine_token');
    activeUser = null;
    activeKey = null;

    document.getElementById('dashboard-container').style.display = 'none';
    document.getElementById('nav-user-area').style.display = 'none';
    document.getElementById('auth-container').style.display = 'block';

    cancelTelegramChallenge();
    showToast('Signed out', 'default');
  }

  // --- Navigation & Tab Switching ---
  function switchTab(tabId) {
    document.querySelectorAll('#main-tabs-bar .hero-tab-item').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.tab === tabId);
    });
    document.querySelectorAll('.tab-pane').forEach(pane => {
      pane.style.display = pane.id === `pane-${tabId}` ? 'flex' : 'none';
    });
  }

  // --- Initial Event Bindings ---
  function initEventBindings() {
    // Auth Mode Segmented Control
    const tabAuthTg = document.getElementById('tab-auth-tg');
    const tabAuthClt = document.getElementById('tab-auth-clt');
    const panelTg = document.getElementById('panel-auth-tg');
    const panelClt = document.getElementById('panel-auth-clt');

    tabAuthTg.addEventListener('click', () => {
      tabAuthTg.classList.add('active');
      tabAuthClt.classList.remove('active');
      panelTg.style.display = 'flex';
      panelClt.style.display = 'none';
    });

    tabAuthClt.addEventListener('click', () => {
      tabAuthClt.classList.add('active');
      tabAuthTg.classList.remove('active');
      panelClt.style.display = 'flex';
      panelTg.style.display = 'none';
    });

    // Telegram Verification Buttons
    document.getElementById('btn-start-verification').addEventListener('click', startTelegramVerification);
    document.getElementById('btn-cancel-challenge').addEventListener('click', cancelTelegramChallenge);

    // Machine Client Login
    document.getElementById('btn-login-clt').addEventListener('click', loginMachineClient);

    // Main Tabs Navigation
    document.querySelectorAll('#main-tabs-bar .hero-tab-item').forEach(btn => {
      btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    // Key Actions
    document.getElementById('btn-generate-key').addEventListener('click', handleGenerateKey);
    document.getElementById('btn-open-rotate-modal').addEventListener('click', () => openModal('modal-confirm-rotate'));
    document.getElementById('btn-confirm-rotate').addEventListener('click', handleRotateKey);
    document.getElementById('btn-cancel-rotate').addEventListener('click', () => closeModal('modal-confirm-rotate'));

    document.getElementById('btn-open-revoke-modal').addEventListener('click', () => openModal('modal-confirm-revoke'));
    document.getElementById('btn-confirm-revoke').addEventListener('click', handleRevokeKey);
    document.getElementById('btn-cancel-revoke').addEventListener('click', () => closeModal('modal-confirm-revoke'));

    document.getElementById('btn-close-reveal-modal').addEventListener('click', () => closeModal('modal-reveal-secret'));

    // Copy Buttons
    document.getElementById('btn-copy-prefix')?.addEventListener('click', async () => {
      const prefix = document.getElementById('active-key-prefix').textContent;
      if (await copyText(prefix)) showToast('Masked prefix copied', 'success');
    });

    document.getElementById('btn-copy-revealed-secret')?.addEventListener('click', async () => {
      const secret = document.getElementById('revealed-secret-text').textContent;
      if (await copyText(secret)) showToast('API Key copied to clipboard!', 'success');
    });

    document.getElementById('btn-copy-code')?.addEventListener('click', async () => {
      const code = document.getElementById('code-snippet-box').textContent;
      if (await copyText(code)) showToast('Code snippet copied', 'success');
    });

    // Feature Toggles
    document.querySelectorAll('.hero-switch-toggle').forEach(el => {
      el.addEventListener('click', () => {
        const feature = el.dataset.feature;
        if (feature) toggleFeature(feature);
      });
    });

    // Quickstart Snippet Tabs
    document.querySelectorAll('#pane-quickstart .hero-tab-item').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('#pane-quickstart .hero-tab-item').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        currentSnippetType = btn.dataset.snippet;
        renderQuickstartCode();
      });
    });

    // Chat Playground
    document.getElementById('btn-send-chat').addEventListener('click', handleSendChat);
    document.getElementById('btn-stop-chat').addEventListener('click', handleStopChat);
    document.getElementById('playground-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSendChat();
      }
    });
    document.getElementById('btn-clear-chat').addEventListener('click', () => {
      const box = document.getElementById('playground-messages');
      box.innerHTML = `
        <div class="chat-msg assistant">
          <div class="chat-bubble">Chat cleared. Select an authorized model alias and send a message.</div>
          <span class="chat-meta">Zyrouter Gateway</span>
        </div>
      `;
    });

    // Logout
    document.getElementById('btn-logout').addEventListener('click', handleLogout);

    // Auto-login if session cookie or stored token exists
    bootstrapDashboard();
  }

  // --- Bootstrap on DOM Ready ---
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initEventBindings);
  } else {
    initEventBindings();
  }
})();

(() => {
  'use strict';

  /* ==========================================================================
     ZYROUTER CLIENT PORTAL · APPLICATION LOGIC
     Blueprint Implementation: Telegram Auth, Machine Client, Realtime Streams,
     Allowed Models, Personal Usage, Interactive Playground & API Guide.
     ========================================================================== */

  // Control & Inference Base Endpoints
  const localControl = location.hostname === '127.0.0.1' ? 'http://127.0.0.1:20128' : 'http://localhost:20128';
  const storedControl = localStorage.getItem('zy_control_base') || '';
  if (location.hostname === 'localhost' && storedControl === 'http://127.0.0.1:20128') {
    localStorage.removeItem('zy_control_base');
  }
  const control = window.__ZYROUTER_CONTROL_BASE__ ||
    (location.hostname === 'localhost' && storedControl === 'http://127.0.0.1:20128' ? localControl : storedControl) ||
    (location.hostname === 'localhost' || location.hostname === '127.0.0.1' ? localControl : '');

  const INFERENCE_BASE = window.__ZYROUTER_API_BASE__ || localStorage.getItem('zy_api_base') || 'https://api.zyvenox.tech';

  // Application State
  let currentAuthMode = 'telegram'; // 'telegram' | 'machine'
  let sessionType = null; // 'telegram' | 'machine' | null
  let machineToken = ''; // held in memory
  let bootstrapAttempt = 0;

  let challenge = '';
  let timerInterval = null;
  let verificationPoll = null;
  let timeLeftSec = 600;

  let globalSource = null;
  let privateSource = null;
  let isStreamPaused = false;

  let activeProfile = null;
  let activePolicy = null;
  let allowedModelsList = [];
  let usageOffset = 0;
  const usageLimit = 10;


  // DOM Helper
  const $ = (id) => document.getElementById(id);

  // Number & Currency Formatters
  const fmt = (v) => Number(v || 0).toLocaleString('id-ID');
  const fmtCurrency = (v) => {
    const num = Number(v || 0);
    return '$' + num.toFixed(4);
  };

  function formatWIB(timestamp) {
    if (!timestamp) return '-';
    try {
      const d = new Date(timestamp);
      return d.toLocaleString('id-ID', {
        timeZone: 'Asia/Jakarta',
        hour12: false,
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      }) + ' WIB';
    } catch {
      return String(timestamp);
    }
  }

  // Toast Notification System
  function showToast(message, type = 'info') {
    const container = $('toastContainer');
    if (!container) return;
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.innerHTML = `<span>${escapeHtml(message)}</span>`;
    container.appendChild(toast);
    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateY(10px)';
      toast.style.transition = 'all 0.25s ease-out';
      setTimeout(() => toast.remove(), 250);
    }, 3200);
  }

  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  // Generic Authenticated API Wrapper
  async function api(path, options = {}) {
    const headers = { ...(options.headers || {}) };
    if (sessionType === 'machine' && machineToken) {
      headers['Authorization'] = `Bearer ${machineToken}`;
    }
    return fetch(`${control}${path}`, {
      ...options,
      credentials: 'include',
      headers,
    });
  }

  // Tab View Navigation
  function showView(viewId) {
    document.querySelectorAll('.view-panel').forEach((panel) => {
      panel.classList.toggle('hidden', panel.id !== viewId);
      panel.classList.toggle('active', panel.id === viewId);
    });

    document.querySelectorAll('.nav-tab').forEach((tab) => {
      const target = tab.dataset.view || tab.dataset.tab;
      const isMatch = target === viewId;
      tab.classList.toggle('active', isMatch);
      tab.setAttribute('aria-selected', isMatch ? 'true' : 'false');
    });

    if (viewId === 'usage') {
      loadUsageHistory();
    }
  }

  /* ==========================================================================
     AUTHENTICATION & VERIFICATION STATE MACHINE (Blueprint 3)
     ========================================================================== */

  // Switch Auth Mode Tabs
  function setAuthMode(mode) {
    currentAuthMode = mode;
    $('modeTelegramTab')?.classList.toggle('active', mode === 'telegram');
    $('modeMachineTab')?.classList.toggle('active', mode === 'machine');
    $('telegramAuthPanel')?.classList.toggle('active', mode === 'telegram');
    $('machineAuthPanel')?.classList.toggle('active', mode === 'machine');
    clearAuthError();
  }

  $('modeTelegramTab')?.addEventListener('click', () => setAuthMode('telegram'));
  $('modeMachineTab')?.addEventListener('click', () => setAuthMode('machine'));

  function showAuthError(msg) {
    const el = $('auth-error');
    if (el) el.textContent = msg || '';
  }

  function clearAuthError() {
    const el = $('auth-error');
    if (el) el.textContent = '';
  }

  // Telegram Verification Flow (Blueprint 3.2)
  async function startVerification() {
    if (challenge) return;
    const startBtn = $('start');
    if (startBtn) startBtn.disabled = true;
    clearAuthError();

    try {
      const res = await api('/api/user/verification/start', { method: 'POST' });
      if (!res.ok) {
        throw new Error('Gagal membuat verifikasi Telegram (HTTP ' + res.status + ')');
      }
      const data = await res.json();
      challenge = data.challengeId;
      sessionStorage.setItem('zy_active_challenge', challenge);
      sessionStorage.setItem('zy_active_challenge_expires', data.expiresAt || new Date(Date.now() + 600000).toISOString());

      // Update UI to Challenge Pending
      $('telegramInitialState')?.classList.add('hidden');
      $('challenge')?.classList.remove('hidden');
      if ($('code')) $('code').textContent = challenge;

      const botUrl = data.telegramDeepLink || `https://t.me/Zyrouter_bot?start=${encodeURIComponent(challenge)}`;
      if ($('botLink')) $('botLink').href = botUrl;
      if ($('botRetryLink')) $('botRetryLink').href = botUrl;

      // Start 10-Minute (600s) Countdown Timer
      timeLeftSec = 600;
      updateTimerDisplay(timeLeftSec);
      clearInterval(timerInterval);
      timerInterval = setInterval(() => {
        timeLeftSec--;
        updateTimerDisplay(timeLeftSec);
        if (timeLeftSec <= 0) {
          cancelVerification();
          showAuthError('Waktu verifikasi telah habis. Silakan buat kode verifikasi baru.');
        }
      }, 1000);

      // Start Polling Verification Status every 2 seconds
      clearInterval(verificationPoll);
      verificationPoll = setInterval(pollVerificationStatus, 2000);
    } catch (err) {
      showAuthError(err.message || 'Gagal memulai verifikasi');
      if (startBtn) startBtn.disabled = false;
    }
  }

  function updateTimerDisplay(seconds) {
    const m = String(Math.floor(seconds / 60)).padStart(2, '0');
    const s = String(seconds % 60).padStart(2, '0');
    const text = `${m}:${s}`;
    if ($('challenge-timer')) $('challenge-timer').textContent = text;
    if ($('timer')) $('timer').textContent = text;
  }

  function renderRestoredChallenge(expiresAt) {
    $('telegramInitialState')?.classList.add('hidden');
    $('challenge')?.classList.remove('hidden');
    if ($('code')) $('code').textContent = challenge;
    const botUrl = `https://t.me/Zyrouter_bot?start=${encodeURIComponent(challenge)}`;
    if ($('botLink')) $('botLink').href = botUrl;
    if ($('botRetryLink')) $('botRetryLink').href = botUrl;
    const remaining = Math.max(0, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 1000));
    timeLeftSec = remaining || 600;
    updateTimerDisplay(timeLeftSec);
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      timeLeftSec--;
      updateTimerDisplay(timeLeftSec);
      if (timeLeftSec <= 0) {
        cancelVerification();
        showAuthError('Waktu verifikasi telah habis. Silakan buat kode baru.');
      }
    }, 1000);
  }

  async function restoreChallenge() {
    const saved = sessionStorage.getItem('zy_active_challenge');
    if (!saved) return;
    challenge = saved;
    renderRestoredChallenge(sessionStorage.getItem('zy_active_challenge_expires') || new Date(Date.now() + 600000).toISOString());
    try {
      const res = await api(`/api/user/verification/${encodeURIComponent(challenge)}`);
      if (res.status === 404 || res.status === 403) {
        cancelVerification();
        showAuthError('Challenge lama sudah tidak valid. Silakan buat challenge baru.');
        return;
      }
      const data = await res.json();
      if (data.status === 'telegram_verified') {
        $('confirmation')?.classList.remove('hidden');
        clearInterval(timerInterval);
        return;
      }
      clearInterval(verificationPoll);
      verificationPoll = setInterval(pollVerificationStatus, 2000);
    } catch {
      clearInterval(verificationPoll);
      verificationPoll = setInterval(pollVerificationStatus, 2000);
    }
  }

  async function pollVerificationStatus() {
    if (!challenge) return;
    try {
      const res = await api(`/api/user/verification/${encodeURIComponent(challenge)}`);
      if (!res.ok) {
        if (res.status === 403 || res.status === 404) {
          clearInterval(verificationPoll);
          sessionStorage.removeItem('zy_active_challenge');
          sessionStorage.removeItem('zy_active_challenge_expires');
          showAuthError('Challenge tidak lagi terikat ke browser ini. Silakan buat kode verifikasi baru.');
        }
        return;
      }
      const data = await res.json();

      if (data.status === 'telegram_verified') {
        // Stop polling, switch to confirmation state
        clearInterval(verificationPoll);
        $('confirmation')?.classList.remove('hidden');
        $('confirmationCode')?.focus();
        showToast('Identitas Telegram terverifikasi! Masukkan kode konfirmasi dari bot.', 'info');
      } else if (data.status === 'completed' || data.status === 'verified') {
        clearInterval(verificationPoll);
        clearInterval(timerInterval);
        history.replaceState(null, '', '#dashboard');
        await loadDashboard();
      }
    } catch {
      // Ignore transient polling network errors
    }
  }

  async function completeVerification() {
    const codeInput = $('confirmationCode');
    const code = codeInput ? codeInput.value.trim() : '';
    if (!code) {
      showAuthError('Silakan masukkan kode konfirmasi dari Telegram bot.');
      return;
    }

    const completeBtn = $('complete');
    if (completeBtn) completeBtn.disabled = true;
    clearAuthError();

    try {
      const res = await api('/api/user/verification/complete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challengeId: challenge,
          confirmationCode: code,
        }),
      });

      if (!res.ok) {
        throw new Error('Kode konfirmasi salah atau sudah kedaluwarsa.');
      }

      // Successful login
      clearInterval(timerInterval);
      clearInterval(verificationPoll);
      challenge = '';
      sessionStorage.removeItem('zy_active_challenge');
      sessionStorage.removeItem('zy_active_challenge_expires');
      sessionType = 'telegram';

      showToast('Verifikasi berhasil! Masuk ke dashboard...', 'success');
      history.replaceState(null, '', '#dashboard');
      await loadDashboard();
    } catch (err) {
      showAuthError(err.message || 'Gagal menyelesaikan verifikasi');
      if (completeBtn) completeBtn.disabled = false;
    }
  }

  function cancelVerification() {
    clearInterval(timerInterval);
    clearInterval(verificationPoll);
    challenge = '';
    sessionStorage.removeItem('zy_active_challenge');
    sessionStorage.removeItem('zy_active_challenge_expires');
    timeLeftSec = 600;

    $('challenge')?.classList.add('hidden');
    $('confirmation')?.classList.add('hidden');
    $('telegramInitialState')?.classList.remove('hidden');
    if ($('start')) $('start').disabled = false;
    if ($('complete')) $('complete').disabled = false;
    if ($('confirmationCode')) $('confirmationCode').value = '';
    clearAuthError();
  }

  // Machine Client Login (Blueprint 3.3)
  async function connectMachineClient() {
    const tokenInput = $('machineTokenInput');
    const token = tokenInput ? tokenInput.value.trim() : '';
    if (!token) {
      showAuthError('Silakan masukkan machine client token (clt_...).');
      return;
    }

    const btn = $('connectMachineBtn');
    if (btn) btn.disabled = true;
    clearAuthError();

    try {
      const res = await fetch(`${control}/api/client/profile`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      if (!res.ok) {
        throw new Error('Token Machine Client tidak valid (HTTP ' + res.status + ')');
      }

      machineToken = token;
      sessionType = 'machine';
      showToast('Machine Client terhubung!', 'success');
      history.replaceState(null, '', '#dashboard');
      await loadDashboard();
    } catch (err) {
      showAuthError(err.message || 'Koneksi machine client gagal');
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  // Logout Flow (Blueprint 2)
  async function logout() {
    try {
      if (sessionType === 'telegram') {
        await api('/api/user/logout', { method: 'POST' });
      }
    } catch {
      // Ignore network failures on logout
    }

    // Teardown SSE streams
    stopStreams();

    sessionType = null;
    machineToken = '';
    activeProfile = null;
    activePolicy = null;
    allowedModelsList = [];
    challenge = '';

    // Reset UI
    $('app')?.classList.add('hidden');
    $('auth')?.classList.remove('hidden');
    $('sessionMenu')?.classList.add('hidden');
    $('secretBanner')?.classList.add('hidden');
    cancelVerification();
    history.replaceState(null, '', '#');
    showToast('Sesi telah berakhir.', 'info');
  }

  /* ==========================================================================
     DASHBOARD DATA LOADER (Blueprint 4, 5, 6, 7)
     ========================================================================== */
  async function loadDashboard() {
    bootstrapAttempt++;

    try {
      if (sessionType === 'machine' || (!sessionType && machineToken)) {
        await loadMachineDashboard();
      } else {
        await loadTelegramDashboard();
      }

      // Unhide Dashboard App & Header Session Menu
      $('auth')?.classList.add('hidden');
      $('app')?.classList.remove('hidden');
      $('sessionMenu')?.classList.remove('hidden');

      // Populate Allowed Models into UI
      renderAllowedModels(allowedModelsList);

      // Start SSE Telemetry Streams
      startGlobalStream();
      startPrivateLogsStream();

      // Show Default Overview Tab
      showView('overview');
    } catch (err) {
      if (bootstrapAttempt <= 1) {
        // Initial unauthenticated state is expected
        $('app')?.classList.add('hidden');
        $('auth')?.classList.remove('hidden');
        $('sessionMenu')?.classList.add('hidden');
      } else {
        showAuthError(err.message || 'Gagal memuat data dashboard');
      }
    }
  }

  // Telegram Dashboard Loader
  async function loadTelegramDashboard() {
    const [pRes, uRes, kRes] = await Promise.all([
      api('/api/user/profile'),
      api('/api/user/usage'),
      api('/api/user/key'),
    ]);

    if (!pRes.ok) {
      throw new Error('Sesi user belum aktif');
    }

    sessionType = 'telegram';
    const profileData = await pRes.json();
    const usageData = uRes.ok ? await uRes.json() : {};
    const keyData = kRes.ok ? await kRes.json() : {};

    activeProfile = profileData;
    allowedModelsList = profileData.allowedAliases || [];

    // Render Identity
    const user = profileData.user || {};
    const tier = profileData.accountType?.name || 'User';
    const username = user.telegramUsername ? `@${user.telegramUsername}` : (user.displayName || 'Telegram User');

    if ($('profile')) $('profile').textContent = `${user.displayName || ''} ${username}`.trim();
    if ($('sessionUserName')) $('sessionUserName').textContent = username;
    if ($('sessionUserTier')) $('sessionUserTier').textContent = tier;
    if ($('overviewTierBadge')) $('overviewTierBadge').textContent = `Tier: ${tier}`;

    // Render Personal Metrics
    if ($('myRequests')) $('myRequests').textContent = fmt(usageData.totalRequests);
    if ($('myTokens')) $('myTokens').textContent = fmt(usageData.totalTokens);
    if ($('myCost')) $('myCost').textContent = fmtCurrency(usageData.totalCost);
    if ($('myAllowedCount')) $('myAllowedCount').textContent = fmt(allowedModelsList.length);

    // Render Key Status
    renderTelegramKeyStatus(keyData.key);
  }

  // Machine Dashboard Loader
  async function loadMachineDashboard() {
    sessionType = 'machine';
    const [pRes, polRes, uRes, kRes] = await Promise.all([
      api('/api/client/profile'),
      api('/api/client/policy'),
      api('/api/client/usage'),
      api('/api/client/keys'),
    ]);

    if (!pRes.ok) {
      throw new Error('Machine client session expired');
    }

    const profileData = await pRes.json();
    const policyData = polRes.ok ? await polRes.json() : {};
    const usageData = uRes.ok ? await uRes.json() : {};
    const keysData = kRes.ok ? await kRes.json() : { keys: [] };

    activeProfile = profileData;
    activePolicy = policyData;
    allowedModelsList = policyData.allowedModels || [];

    // Render Identity
    const name = profileData.name || 'Machine Client';
    if ($('profile')) $('profile').textContent = `${name} [Machine]`;
    if ($('sessionUserName')) $('sessionUserName').textContent = name;
    if ($('sessionUserTier')) $('sessionUserTier').textContent = 'Machine';
    if ($('overviewTierBadge')) $('overviewTierBadge').textContent = `Policy: ${policyData.name || 'Default'}`;

    // Render Personal Metrics
    if ($('myRequests')) $('myRequests').textContent = fmt(usageData.totalRequests);
    if ($('myTokens')) $('myTokens').textContent = fmt(usageData.totalTokens);
    if ($('myCost')) $('myCost').textContent = fmtCurrency(usageData.totalCost);
    if ($('myAllowedCount')) $('myAllowedCount').textContent = fmt(allowedModelsList.length);

    // Render Machine Keys
    renderMachineKeys(keysData.keys || []);
  }

  /* ==========================================================================
     API KEY MANAGEMENT (Blueprint 5)
     ========================================================================== */
  function renderTelegramKeyStatus(key) {
    const keyInfoEl = $('keyInfo');
    const copyPrefixBtn = $('copyPrefixBtn');
    const generateBtn = $('generate');
    const rotateBtn = $('rotate');
    const revokeBtn = $('revoke');
    const keyBadge = $('keyBadge');
    const overviewKeyBadge = $('overviewKeyBadge');

    if (key) {
      const prefix = key.keyPrefix || key.prefix || 'zy_...';
      if (keyInfoEl) keyInfoEl.textContent = `${prefix}••••••••••••••••`;
      if (copyPrefixBtn) {
        copyPrefixBtn.classList.remove('hidden');
        copyPrefixBtn.onclick = () => copyText(prefix, 'Prefix kunci disalin');
      }
      if (generateBtn) generateBtn.disabled = true;
      if (rotateBtn) rotateBtn.disabled = false;
      if (revokeBtn) revokeBtn.disabled = false;

      if (keyBadge) {
        keyBadge.className = 'status-badge live';
        keyBadge.textContent = '● KUNCI AKTIF';
      }
      if (overviewKeyBadge) overviewKeyBadge.textContent = 'API Key: Aktif';

      // Fill in-memory playground key placeholder
      const pgKey = $('playground-api-key');
      if (pgKey && !pgKey.value) {
        pgKey.placeholder = `${prefix}•••••••• (Aktif)`;
      }
    } else {
      if (keyInfoEl) keyInfoEl.textContent = 'Belum ada API key aktif';
      if (copyPrefixBtn) copyPrefixBtn.classList.add('hidden');
      if (generateBtn) generateBtn.disabled = false;
      if (rotateBtn) rotateBtn.disabled = true;
      if (revokeBtn) revokeBtn.disabled = true;

      if (keyBadge) {
        keyBadge.className = 'status-badge';
        keyBadge.textContent = '○ TIDAK ADA KUNCI';
      }
      if (overviewKeyBadge) overviewKeyBadge.textContent = 'API Key: Belum Ada';
    }
  }

  function renderMachineKeys(keys) {
    const container = $('machineKeysContainer');
    const list = $('machineKeysList');
    if (!container || !list) return;

    container.classList.remove('hidden');
    if (keys.length === 0) {
      list.innerHTML = '<tr><td colspan="4" class="text-center text-muted">Belum ada API key machine client.</td></tr>';
      return;
    }

    list.innerHTML = keys.map((k) => `
      <tr>
        <td><strong>${escapeHtml(k.name || 'Key')}</strong></td>
        <td><code>${escapeHtml(k.prefix || k.keyPrefix || 'zy_...')}</code></td>
        <td>${formatWIB(k.createdAt)}</td>
        <td>
          <button class="btn-cyber-danger btn-xs btn-revoke-machine" data-id="${escapeHtml(k.id)}">Revoke</button>
        </td>
      </tr>
    `).join('');

    list.querySelectorAll('.btn-revoke-machine').forEach((btn) => {
      btn.onclick = () => openConfirmModal(
        'Revoke Machine Key',
        'Apakah Anda yakin ingin mencabut key ini? Semua aplikasi yang menggunakannya akan gagal terhubung.',
        async () => {
          await api(`/api/client/keys/${encodeURIComponent(btn.dataset.id)}`, { method: 'DELETE' });
          showToast('Machine Key berhasil dicabut.', 'info');
          await loadDashboard();
        }
      );
    });
  }

  function revealOneTimeSecret(secret) {
    const banner = $('secretBanner');
    const secretEl = $('secret');
    if (banner && secretEl) {
      secretEl.textContent = secret;
      banner.classList.remove('hidden');
      banner.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      showToast('API Key baru berhasil diterbitkan! Amankan secret sekarang.', 'success');

      // Update in-memory playground key
      const pgKey = $('playground-api-key');
      if (pgKey) pgKey.value = secret;
    }
  }

  $('dismissSecretBtn')?.addEventListener('click', () => {
    $('secretBanner')?.classList.add('hidden');
  });

  $('copySecretBtn')?.addEventListener('click', () => {
    const text = $('secret')?.textContent || '';
    if (text) copyText(text, 'Secret API Key berhasil disalin!');
  });

  // Key Actions
  $('generate')?.addEventListener('click', async () => {
    const endpoint = sessionType === 'machine' ? '/api/client/keys' : '/api/user/key';
    const body = sessionType === 'machine' ? JSON.stringify({ name: 'Machine Key' }) : undefined;
    try {
      const res = await api(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body,
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || `Gagal membuat key (HTTP ${res.status})`);
      }
      const data = await res.json();
      if (data.key) revealOneTimeSecret(data.key);
      await loadDashboard();
    } catch (err) {
      showToast(err.message || 'Gagal generate key', 'error');
    }
  });

  $('rotate')?.addEventListener('click', () => {
    openConfirmModal(
      'Rotate Gateway API Key',
      'Tindakan ini akan membatalkan API Key yang lama secara permanen dan menerbitkan key baru. Lanjutkan?',
      async () => {
        try {
          const res = await api('/api/user/key/rotate', { method: 'POST' });
          if (!res.ok) throw new Error('Gagal rotate key');
          const data = await res.json();
          if (data.key) revealOneTimeSecret(data.key);
          await loadDashboard();
        } catch (err) {
          showToast(err.message || 'Gagal rotate key', 'error');
        }
      }
    );
  });

  $('revoke')?.addEventListener('click', () => {
    openConfirmModal(
      'Revoke Gateway API Key',
      'Apakah Anda yakin ingin mencabut (revoke) API Key ini? Akses inference menggunakan kunci ini akan langsung ditolak.',
      async () => {
        try {
          const res = await api('/api/user/key', { method: 'DELETE' });
          if (!res.ok) throw new Error('Gagal mencabut key');
          showToast('API Key telah dicabut.', 'info');
          await loadDashboard();
        } catch (err) {
          showToast(err.message || 'Gagal revoke key', 'error');
        }
      }
    );
  });

  /* ==========================================================================
     ALLOWED MODELS (Blueprint 6)
     ========================================================================== */
  function renderAllowedModels(models) {
    const container = $('modelsList');
    if (!container) return;

    if (!models || models.length === 0) {
      container.innerHTML = '<div class="loading-placeholder">Tidak ada alias model yang diizinkan untuk akun Anda.</div>';
      return;
    }

    container.innerHTML = models.map((alias) => `
      <div class="model-alias-card">
        <div class="model-card-top">
          <div>
            <div class="model-alias-name">${escapeHtml(alias)}</div>
            <div class="model-caps-row">
              <span class="cap-tag">Chat</span>
              <span class="cap-tag">Stream</span>
              <span class="cap-tag">OpenAI Spec</span>
            </div>
          </div>
          <span class="status-badge live">● Aktif</span>
        </div>
        <div class="model-actions-row">
          <button class="btn-cyber-outline btn-xs btn-copy-alias" data-alias="${escapeHtml(alias)}">
            Salin Alias
          </button>
          <button class="btn-cyber-outline btn-xs btn-copy-curl" data-alias="${escapeHtml(alias)}">
            Salin cURL
          </button>
        </div>
      </div>
    `).join('');

    // Attach Event Listeners to Model Cards
    container.querySelectorAll('.btn-copy-alias').forEach((btn) => {
      btn.onclick = () => copyText(btn.dataset.alias, `Alias "${btn.dataset.alias}" disalin!`);
    });


    container.querySelectorAll('.btn-copy-curl').forEach((btn) => {
      btn.onclick = () => {
        const curl = `curl https://api.zyvenox.tech/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer $ZYROUTER_API_KEY" \\\n  -d '{\n    "model": "${btn.dataset.alias}",\n    "messages": [{"role": "user", "content": "Hello!"}]\n  }'`;
        copyText(curl, 'cURL request disalin ke clipboard!');
      };
    });
  }

  // Model Search Filter
  $('modelSearchInput')?.addEventListener('input', (e) => {
    const q = (e.target.value || '').toLowerCase().trim();
    const filtered = allowedModelsList.filter((m) => m.toLowerCase().includes(q));
    renderAllowedModels(filtered);
  });


  /* ==========================================================================
     PERSONAL USAGE HISTORY (Blueprint 7)
     ========================================================================== */
  async function loadUsageHistory() {
    const tableBody = $('usageTableBody');
    if (!tableBody) return;

    const endpoint = sessionType === 'machine'
      ? `/api/client/logs?limit=${usageLimit}&offset=${usageOffset}`
      : `/api/user/logs?limit=${usageLimit}&offset=${usageOffset}`;

    try {
      const res = await api(endpoint);
      if (!res.ok) throw new Error('Gagal memuat log pemakaian');
      const data = await res.json();
      const items = data.items || [];

      if (items.length === 0) {
        tableBody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">Belum ada riwayat pemakaian inference.</td></tr>';
      } else {
        tableBody.innerHTML = items.map((item) => {
          const statusClass = (item.statusCode >= 200 && item.statusCode < 300) ? 'success' : (item.statusCode >= 400 && item.statusCode < 500 ? 'warn' : 'error');
          const totalTok = item.totalTokens || ((item.promptTokens || 0) + (item.completionTokens || 0));
          return `
            <tr>
              <td>${formatWIB(item.timestamp)}</td>
              <td><code>${escapeHtml((item.requestId || item.id || '').slice(0, 12))}</code></td>
              <td><strong>${escapeHtml(item.model || item.modelAlias || '-')}</strong></td>
              <td><span class="status-cell-tag ${statusClass}">${item.statusCode || 200}</span></td>
              <td>${item.durationMs != null ? `${item.durationMs}ms` : '-'}</td>
              <td>${fmt(item.promptTokens)}</td>
              <td>${fmt(item.completionTokens)}</td>
              <td><strong>${fmt(totalTok)}</strong></td>
            </tr>
          `;
        }).join('');
      }

      // Pagination Controls
      const pageNum = Math.floor(usageOffset / usageLimit) + 1;
      if ($('usagePaginationInfo')) $('usagePaginationInfo').textContent = `Halaman ${pageNum}`;
      if ($('prevUsagePageBtn')) $('prevUsagePageBtn').disabled = usageOffset === 0;
      if ($('nextUsagePageBtn')) $('nextUsagePageBtn').disabled = items.length < usageLimit;
    } catch {
      tableBody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">Gagal mengambil log pemakaian.</td></tr>';
    }
  }

  $('prevUsagePageBtn')?.addEventListener('click', () => {
    if (usageOffset >= usageLimit) {
      usageOffset -= usageLimit;
      loadUsageHistory();
    }
  });

  $('nextUsagePageBtn')?.addEventListener('click', () => {
    usageOffset += usageLimit;
    loadUsageHistory();
  });

  $('refreshUsageBtn')?.addEventListener('click', () => {
    loadUsageHistory();
    showToast('Data pemakaian diperbarui.', 'info');
  });

  /* ==========================================================================
     GLOBAL TELEMETRY STREAM & PRIVATE LOG STREAM (Blueprint 8 & 9)
     ========================================================================== */
  function renderStreamEvent(e) {
    if (isStreamPaused) return;
    const list = $('logsList');
    if (!list) return;

    // Remove empty state placeholder if present
    const emptyState = list.querySelector('.stream-empty-state');
    if (emptyState) emptyState.remove();

    const row = document.createElement('div');
    row.className = 'stream-row';

    const timeStr = new Date(e.timestamp || Date.now()).toLocaleTimeString('id-ID', {
      timeZone: 'Asia/Jakarta',
      hour12: false,
    });
    const statusClass = e.status === 'error' ? 'error' : 'success';
    const totalTokens = e.totalTokens || ((e.promptTokens || 0) + (e.completionTokens || 0));

    row.innerHTML = `
      <span class="stream-time">${timeStr} WIB</span>
      <span class="stream-model">${escapeHtml(e.model || 'global-alias')}</span>
      <span class="status-cell-tag ${statusClass}">${escapeHtml(e.status || '200')}</span>
      <span class="stream-tokens">${fmt(totalTokens)} tok</span>
      <span class="stream-time">${e.durationMs != null ? `${e.durationMs}ms` : ''}</span>
    `;

    list.prepend(row);

    // Bounded memory: keep max 100 rows
    while (list.children.length > 100) {
      list.lastElementChild.remove();
    }
  }

  function startGlobalStream() {
    if (globalSource) return;

    const streamUrl = sessionType === 'machine'
      ? `${control}/api/client/global-usage/stream`
      : `${control}/api/user/global-usage/stream`;

    setStreamState('CONNECTING');

    globalSource = new EventSource(streamUrl, { withCredentials: true });

    globalSource.onopen = () => {
      setStreamState('LIVE');
    };

    globalSource.onerror = () => {
      setStreamState('RECONNECTING');
    };

    globalSource.onmessage = (msg) => {
      try {
        const data = JSON.parse(msg.data);
        if ($('globalRequests')) $('globalRequests').textContent = fmt(data.totalRequests);
        if ($('globalTokens')) $('globalTokens').textContent = fmt(data.totalTokens);
        if (Array.isArray(data.recent)) {
          data.recent.forEach(renderStreamEvent);
        }
      } catch {
        // Ignore JSON parse errors on ping/keep-alive
      }
    };
  }

  function startPrivateLogsStream() {
    if (privateSource) return;

    const streamUrl = sessionType === 'machine'
      ? `${control}/api/client/logs/stream`
      : `${control}/api/user/logs/stream`;

    privateSource = new EventSource(streamUrl, { withCredentials: true });

    privateSource.addEventListener('snapshot', (evt) => {
      // Snapshot received
    });

    privateSource.addEventListener('log', (evt) => {
      try {
        const logEntry = JSON.parse(evt.data);
        renderStreamEvent(logEntry);
      } catch {}
    });

    privateSource.onerror = () => {
      // Private stream reconnect handled by browser
    };
  }

  function setStreamState(state) {
    const el = $('streamState');
    if (!el) return;
    el.textContent = state;
    el.className = `status-pill ${state.toLowerCase()}`;
  }

  function stopStreams() {
    if (globalSource) {
      globalSource.close();
      globalSource = null;
    }
    if (privateSource) {
      privateSource.close();
      privateSource = null;
    }
  }

  $('pauseStreamBtn')?.addEventListener('click', () => {
    isStreamPaused = !isStreamPaused;
    const btn = $('pauseStreamBtn');
    if (btn) btn.textContent = isStreamPaused ? 'Lanjutkan Stream' : 'Jeda Stream';
    setStreamState(isStreamPaused ? 'PAUSED' : 'LIVE');
    showToast(isStreamPaused ? 'Stream dijeda.' : 'Stream dilanjutkan.', 'info');
  });

  $('clearLogsBtn')?.addEventListener('click', () => {
    const list = $('logsList');
    if (list) {
      list.innerHTML = '<div class="stream-empty-state"><span class="stream-radar-icon"></span><p>Tampilan dibersihkan. Menunggu event berikutnya...</p></div>';
    }
    showToast('Tampilan stream dibersihkan.', 'info');
  });

  $('reconnectStreamBtn')?.addEventListener('click', () => {
    stopStreams();
    startGlobalStream();
    startPrivateLogsStream();
    showToast('Menghubungkan ulang stream telemetry...', 'info');
  });

  /* ==========================================================================
     API GUIDE CODE TABS & SNIPPET COPY (Blueprint 11)
     ========================================================================== */
  document.querySelectorAll('.code-tab-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const lang = btn.dataset.lang;
      document.querySelectorAll('.code-tab-btn').forEach((b) => b.classList.remove('active'));
      document.querySelectorAll('.code-snippet-panel').forEach((p) => p.classList.remove('active'));
      btn.classList.add('active');
      $(`snippet-${lang}`)?.classList.add('active');
    });
  });

  document.querySelectorAll('[data-copy-target]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const targetId = btn.dataset.copyTarget;
      const codeEl = $(targetId);
      if (codeEl) copyText(codeEl.textContent, 'Kode berhasil disalin!');
    });
  });

  document.querySelectorAll('[data-copy]').forEach((btn) => {
    btn.addEventListener('click', () => {
      copyText(btn.dataset.copy, 'Teks berhasil disalin!');
    });
  });

  /* ==========================================================================
     CONFIRMATION MODAL SYSTEM
     ========================================================================== */
  let activeConfirmAction = null;

  function openConfirmModal(title, description, onConfirm) {
    const modal = $('confirmModal');
    if (!modal) return;
    if ($('modalTitle')) $('modalTitle').textContent = title;
    if ($('modalDesc')) $('modalDesc').textContent = description;
    activeConfirmAction = onConfirm;
    modal.classList.remove('hidden');
  }

  function closeConfirmModal() {
    $('confirmModal')?.classList.add('hidden');
    activeConfirmAction = null;
  }

  $('modalCancelBtn')?.addEventListener('click', closeConfirmModal);
  $('modalConfirmBtn')?.addEventListener('click', async () => {
    if (activeConfirmAction) {
      const action = activeConfirmAction;
      closeConfirmModal();
      await action();
    }
  });

  /* ==========================================================================
     CLIPBOARD UTILITY (Cross-Browser Safe)
     ========================================================================== */
  async function copyText(value, successMessage = 'Disalin ke clipboard') {
    if (!value) return;
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(value);
      } else {
        const ta = document.createElement('textarea');
        ta.value = value;
        ta.style.position = 'fixed';
        ta.style.left = '-9999px';
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        document.execCommand('copy');
        ta.remove();
      }
      showToast(successMessage, 'success');
    } catch {
      showToast('Gagal menyalin ke clipboard', 'error');
    }
  }

  /* ==========================================================================
     GLOBAL EVENT LISTENERS & INITIALIZATION
     ========================================================================== */
  $('start')?.addEventListener('click', startVerification);
  $('complete')?.addEventListener('click', completeVerification);
  $('cancelChallengeBtn')?.addEventListener('click', cancelVerification);
  $('copyChallengeBtn')?.addEventListener('click', () => {
    if (challenge) copyText(challenge, 'Kode verifikasi disalin!');
  });

  $('connectMachineBtn')?.addEventListener('click', connectMachineClient);
  $('toggleMachineTokenVisibility')?.addEventListener('click', () => {
    const input = $('machineTokenInput');
    if (input) input.type = input.type === 'password' ? 'text' : 'password';
  });

  $('logoutBtn')?.addEventListener('click', logout);
  $('brandLink')?.addEventListener('click', (e) => {
    e.preventDefault();
    if (sessionType) showView('overview');
  });


  // Tab View Click Listeners
  document.querySelectorAll('[data-view], [data-tab]').forEach((b) => {
    b.addEventListener('click', () => {
      const view = b.dataset.view || b.dataset.tab;
      showView(view);
    });
  });

  // Enter key trigger on confirmation input
  $('confirmationCode')?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      completeVerification();
    }
  });

  // Enter key trigger on machine token input
  $('machineTokenInput')?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      connectMachineClient();
    }
  });

  // Restore an active verification challenge before the unauthenticated bootstrap.
  restoreChallenge().finally(() => loadDashboard().catch(() => {}));
})();

const views = {
  overview: ['SIGNAL ROOM', 'See the signal.', 'One calm surface for the traffic, health, and policy decisions inside your AI gateway.', 'Refresh connection'],
  providers: ['NODES', 'Provider nodes', 'Manage live connections to your routing fabric.', 'Add connection'],
  orchestrator: ['FLOWS', 'Combo orchestrator', 'Compose fallback, round-robin, sticky, and fusion strategies.', 'Create combo'],
  keys: ['KEYS', 'API key governance', 'Control access with model, prefix, and provider restrictions.', 'Create API key'],
  usage: ['LEDGER', 'Usage ledger', 'Inspect token volume and cost from the SQLite rollup.', 'Export ledger'],
  logs: ['TRACE', 'Stream inspector', 'Observe translator events and request traces as they happen.', 'Connect stream'],
  pools: ['POOLS', 'Proxy pools', 'Review deployable proxy pools and their test state.', 'Add pool'],
  aliases: ['ALIASES', 'Model aliases', 'Map client-facing model names to backend model routes.', 'Create alias'],
  settings: ['SETUP', 'System settings', 'Configure the gateway without leaving the control plane.', 'Open settings']
};

const overview = document.querySelector('#view-overview');
const generic = document.querySelector('#view-generic');
const nav = document.querySelector('#nav-list');
const breadcrumb = document.querySelector('#breadcrumb');
const content = document.querySelector('#generic-content');
const apiBase = window.ZYROUTER_API_BASE || '';
let activeStream = null;
let dashboardAuthenticated = false;
const providerAccountPages = new Map();
function hasDashboardAccess() {
  return dashboardAuthenticated || Boolean(getAuthToken());
}
function getAuthToken() {
  return window.localStorage.getItem('zyrouter.sessionToken') || window.localStorage.getItem('zyrouter.apiKey') || '';
}

function setAuthToken(token) {
  if (token) {
    window.localStorage.setItem('zyrouter.sessionToken', token);
    window.localStorage.setItem('zyrouter.apiKey', token);
  } else {
    window.localStorage.removeItem('zyrouter.sessionToken');
    window.localStorage.removeItem('zyrouter.apiKey');
  }
}

const getHeaders = () => {
  const apiKey = getAuthToken();
  return apiKey ? { Authorization: `Bearer ${apiKey}` } : {};
};

async function copyText(value) {
  const text = String(value ?? '');
  if (!text) throw new Error('Nothing to copy');
  if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    await navigator.clipboard.writeText(text);
    return;
  }

  // Clipboard API is unavailable on plain HTTP VPS origins. Use the browser
  // compatibility path so model IDs and keys still work without HTTPS.
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.select();
  const copied = document.execCommand('copy');
  area.remove();
  if (!copied) throw new Error('Clipboard is unavailable in this browser');
}

const apiKey = getAuthToken();
const headers = getHeaders();

const request = (path) => fetch(`${apiBase}${path}`, { headers: getHeaders(), credentials: 'same-origin' }).then(async (response) => {
  const text = await response.text();
  let payload = {};
  try {
    payload = text ? JSON.parse(text) : {};
  } catch {
    payload = { data: text, message: text };
  }
  if (!response.ok) {
    let errorMsg = '';
    if (typeof payload.error === 'string') {
      errorMsg = payload.error;
    } else if (payload.error && typeof payload.error.message === 'string') {
      errorMsg = payload.error.message;
    } else if (typeof payload.message === 'string') {
      errorMsg = payload.message;
    } else if (payload.error && typeof payload.error === 'object') {
      errorMsg = JSON.stringify(payload.error);
    }
    if (!errorMsg) errorMsg = text || `${response.status} ${response.statusText}`;
    const err = new Error(errorMsg);
    err.status = response.status;
    err.payload = payload;
    throw err;
  }
  return payload;
});
function isItemActive(item) {
  if (!item) return false;
  return item.isActive === 1 || item.isActive === true || item.isActive === '1';
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>'"]/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' })[character]);
}

function maskKey(value) {
  const key = String(value || '');
  return key.length > 10 ? `${key.slice(0, 7)}...${key.slice(-4)}` : key || '--';
}

function formatTokenCount(value) {
  const amount = Number(value) || 0;
  const absolute = Math.abs(amount);
  if (absolute >= 1e9) return `${(amount / 1e9).toFixed(absolute >= 1e10 ? 1 : 2)}B`;
  if (absolute >= 1e6) return `${(amount / 1e6).toFixed(absolute >= 1e7 ? 1 : 2)}M`;
  if (absolute >= 1e3) return `${(amount / 1e3).toFixed(absolute >= 1e5 ? 1 : 2)}K`;
  return Math.round(amount).toLocaleString('en-US');
}
function emptySurface(message = 'No data connected') {
  return `<div class="card generic-empty"><span class="empty-symbol large">+</span><h2>${escapeHtml(message)}</h2><p>Data on this surface is read from the Go engine and SQLite database.</p></div>`;
}

function renderFullLoginGate() {
  const existing = document.querySelector('#full-login-overlay');
  if (existing) return;

  const overlay = document.createElement('div');
  overlay.id = 'full-login-overlay';
  overlay.className = 'modal-backdrop';
  overlay.style.cssText = `
    position: fixed;
    inset: 0;
    z-index: 99999;
    display: grid;
    place-items: center;
    background: radial-gradient(ellipse at 50% 20%, rgba(200,255,99,0.06), transparent 50%), #040609;
    backdrop-filter: blur(12px);
  `;
  overlay.innerHTML = `
    <div class="cyber-modal-card" style="max-width: 400px; width: 90%; padding: 32px 28px; text-align: center; background: rgba(14, 18, 26, 0.92); border: 1px solid rgba(200, 255, 99, 0.35); box-shadow: 0 24px 60px rgba(0,0,0,0.85), 0 0 30px rgba(200,255,99,0.12); border-radius: 14px;">
      <div style="display: inline-flex; width: 52px; height: 52px; border-radius: 50%; background: var(--lime-dim); border: 1px solid rgba(200, 255, 99, 0.4); align-items: center; justify-content: center; margin-bottom: 14px; box-shadow: 0 0 20px rgba(200,255,99,0.2);">
        <span class="material-symbols-outlined" style="font-size: 26px; color: var(--lime);">lock</span>
      </div>
      <span class="kicker" style="font-size: 9px; color: var(--lime); letter-spacing: 1.5px;">ZYROUTER CONTROL PLANE</span>
      <h2 style="font-size: 18px; margin: 4px 0 6px; color: var(--text-bright); font-weight: 700;">Sign in to Dashboard</h2>
      <p style="font-size: 11.5px; color: var(--muted); margin: 0 0 20px; line-height: 1.45;">
        Enter your master password to access the AI gateway routing fabric, proxy pools, and telemetry ledger.
      </p>
      <form id="global-auth-gate-form" style="display: grid; gap: 14px; text-align: left;">
        <label style="font-size: 10px; font-family: var(--mono); color: var(--muted); text-transform: uppercase;">
          Dashboard Password
          <input type="password" id="global-auth-gate-password" placeholder="Enter dashboard password" style="width: 100%; padding: 10px 12px; font-size: 12.5px; font-family: var(--mono); background: #06090d; border: 1px solid var(--line); border-radius: 6px; color: var(--text); margin-top: 4px; box-sizing: border-box;" required autocomplete="current-password" autofocus />
        </label>
        <button class="solid-button" type="submit" id="btn-global-login-submit" style="justify-content: center; padding: 11px 16px; font-size: 12.5px; font-weight: 600;">
          Unlock Control Center &rarr;
        </button>
        <p id="global-auth-error" style="font-size: 11px; color: var(--danger); margin: 0; text-align: center; min-height: 14px; font-family: var(--mono);"></p>
        <p style="font-size: 10px; color: #5a6e82; margin: 0; text-align: center;">
          Password is configured by the operator via <code>INITIAL_PASSWORD</code> &bull; Encrypted in SQLite
        </p>
      </form>
    </div>
  `;

  document.body.appendChild(overlay);
  const form = overlay.querySelector('#global-auth-gate-form');
  const input = overlay.querySelector('#global-auth-gate-password');
  const errEl = overlay.querySelector('#global-auth-error');
  const submitBtn = overlay.querySelector('#btn-global-login-submit');

  input.focus();

  form.onsubmit = async (e) => {
    e.preventDefault();
    const pwd = input.value.trim();
    if (!pwd) return;

    submitBtn.disabled = true;
    submitBtn.innerHTML = '<span class="spinner-icon"></span> Authenticating...';
    errEl.textContent = '';

    try {
      const res = await fetch(`${apiBase}/api/auth/login`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pwd })
      });
      const resText = await res.text();
      let data = {};
      try { data = JSON.parse(resText); } catch {}
      if (!res.ok) throw new Error(data.error?.message || data.error || data.message || resText || 'Authentication failed');

      dashboardAuthenticated = true;
      // Session is held by the HttpOnly cookie, not JavaScript storage.
      window.localStorage.removeItem('zyrouter.sessionToken');
      window.localStorage.removeItem('zyrouter.apiKey');
      showToast('Welcome back! Dashboard unlocked.', 'success');
      overlay.remove();

      // Reload view with fresh credentials
      const currentView = window.location.hash.slice(1) || 'overview';
      setView(currentView);
      if (currentView === 'overview') {
        loadOverview();
      }
    } finally {
      submitBtn.disabled = false;
      submitBtn.innerHTML = 'Unlock Control Center &rarr;';
    }
  };
}

// ─────────────────────────────────────────────────────────────
// CYBER MODAL & TOAST NOTIFICATION HELPERS
// ─────────────────────────────────────────────────────────────
function showToast(message, type = 'info') {
  let container = document.querySelector('#toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    document.body.appendChild(container);
  }

  const icons = {
    success: '✓',
    error: '✕',
    info: 'ℹ'
  };

  const toast = document.createElement('div');
  toast.className = `cyber-toast ${type}`;
  toast.innerHTML = `
    <span class="toast-icon" style="font-weight:bold; font-size:13px;">${icons[type] || 'ℹ'}</span>
    <span style="flex:1;">${escapeHtml(message)}</span>
  `;

  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(20px)';
    setTimeout(() => toast.remove(), 200);
  }, 3200);
}

function showPromptModal({ title = 'Prompt', kicker = 'INPUT REQUIRED', message = '', label = 'Value', defaultValue = '', placeholder = '', confirmText = 'Save' } = {}) {
  return new Promise((resolve) => {
    const backdrop = document.createElement('div');
    backdrop.className = 'modal-backdrop';
    backdrop.innerHTML = `
      <div class="cyber-modal-card">
        <div class="cyber-modal-head">
          <div>
            <span class="kicker" style="font-size:8px;">${escapeHtml(kicker)}</span>
            <h3>${escapeHtml(title)}</h3>
          </div>
          <button type="button" class="cancel-button" id="btn-modal-close" style="padding:2px 6px;">&times;</button>
        </div>
        <form id="cyber-prompt-form">
          <div class="cyber-modal-body">
            ${message ? `<p>${escapeHtml(message)}</p>` : ''}
            <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
              ${escapeHtml(label)}
              <input name="promptValue" id="cyber-modal-input" value="${escapeHtml(defaultValue)}" placeholder="${escapeHtml(placeholder)}" required autocomplete="off" />
            </label>
            <p class="form-error" style="font-size:10.5px; color:var(--danger); margin:0;"></p>
          </div>
          <div class="cyber-modal-actions">
            <button type="button" class="cancel-button" id="btn-modal-cancel">Cancel</button>
            <button type="submit" class="solid-button" id="btn-modal-submit">${escapeHtml(confirmText)}</button>
          </div>
        </form>
      </div>
    `;

    document.body.appendChild(backdrop);
    const input = backdrop.querySelector('#cyber-modal-input');
    input.focus();
    input.select();

    const cleanup = (val) => {
      backdrop.remove();
      resolve(val);
    };

    backdrop.querySelector('#btn-modal-close').onclick = () => cleanup(null);
    backdrop.querySelector('#btn-modal-cancel').onclick = () => cleanup(null);
    backdrop.onclick = (e) => { if (e.target === backdrop) cleanup(null); };

    backdrop.querySelector('#cyber-prompt-form').onsubmit = (e) => {
      e.preventDefault();
      const val = input.value.trim();
      cleanup(val);
    };
  });
}

function showConfirmModal({ title = 'Confirm Action', kicker = 'CONFIRMATION', message = 'Are you sure you want to proceed?', confirmText = 'Confirm', danger = false } = {}) {
  return new Promise((resolve) => {
    const backdrop = document.createElement('div');
    backdrop.className = 'modal-backdrop';
    backdrop.innerHTML = `
      <div class="cyber-modal-card">
        <div class="cyber-modal-head">
          <div>
            <span class="kicker" style="font-size:8px; color:${danger ? 'var(--danger)' : 'var(--lime)'};">${escapeHtml(kicker)}</span>
            <h3>${escapeHtml(title)}</h3>
          </div>
          <button type="button" class="cancel-button" id="btn-confirm-close" style="padding:2px 6px;">&times;</button>
        </div>
        <div class="cyber-modal-body">
          <p>${escapeHtml(message)}</p>
        </div>
        <div class="cyber-modal-actions">
          <button type="button" class="cancel-button" id="btn-confirm-cancel">Cancel</button>
          <button type="button" class="${danger ? 'danger-button' : 'solid-button'}" id="btn-confirm-ok">${escapeHtml(confirmText)}</button>
        </div>
      </div>
    `;

    document.body.appendChild(backdrop);

    const cleanup = (confirmed) => {
      backdrop.remove();
      resolve(confirmed);
    };

    backdrop.querySelector('#btn-confirm-close').onclick = () => cleanup(false);
    backdrop.querySelector('#btn-confirm-cancel').onclick = () => cleanup(false);
    backdrop.querySelector('#btn-confirm-ok').onclick = () => cleanup(true);
    backdrop.onclick = (e) => { if (e.target === backdrop) cleanup(false); };
  });
}
// ─────────────────────────────────────────────────────────────
// COMPREHENSIVE 9ROUTER PROVIDER CATALOG DEFINITIONS
// ─────────────────────────────────────────────────────────────
const KNOWN_PROVIDER_CATALOG = [
  {
    "id": "custom-openai-compatible",
    "name": "OpenAI Compatible",
    "desc": "Any OpenAI-compatible gateway / vLLM / SGLang",
    "icon": "🔌",
    "category": "custom",
    "authType": "custom-openai",
    "defaultModels": [
      "custom-model-1"
    ],
    "alias": "custom-openai-compatible"
  },
  {
    "id": "custom-anthropic-compatible",
    "name": "Anthropic Compatible",
    "desc": "Any Anthropic Messages API proxy endpoint",
    "icon": "🔌",
    "category": "custom",
    "authType": "custom-anthropic",
    "defaultModels": [
      "claude-custom-1"
    ],
    "alias": "custom-anthropic-compatible"
  },
  {
    "id": "custom-embedding",
    "name": "Custom Embedding API",
    "desc": "Self-hosted or cloud embedding proxy endpoint",
    "icon": "📐",
    "category": "custom",
    "authType": "custom-openai",
    "defaultModels": [
      "text-embedding-3-small"
    ],
    "alias": "custom-embedding"
  },
  {
    "id": "antigravity",
    "name": "Google Cloud / Antigravity",
    "desc": "Google Cloud OAuth Device Flow & IDE Code Assist tokens",
    "icon": "🛡️",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "gemini-3.8-flash-high",
      "gemini-3.8-flash-medium",
      "gemini-3.8-flash-low",
      "gemini-3.8-flash",
      "gemini-3.7-flash-high",
      "gemini-3.7-flash-medium",
      "gemini-3.7-flash-low",
      "gemini-3.7-flash",
      "gemini-3.6-flash-high",
      "gemini-3.6-flash-medium",
      "gemini-3.6-flash-low",
      "gemini-3.5-flash-high",
      "gemini-3-flash-agent",
      "gemini-3.5-flash-low",
      "gemini-3.5-flash-extra-low",
      "gemini-pro-agent",
      "gemini-3.1-pro-low",
      "claude-sonnet-4-6",
      "claude-opus-4-6-thinking",
      "claude-3-7-sonnet",
      "claude-3-5-sonnet",
      "gpt-oss-120b-medium",
      "gemini-3-flash",
      "gemini-3.1-flash-image"
    ],
    "alias": "ag"
  },
  {
    "id": "codex",
    "name": "OpenAI Codex",
    "desc": "OpenAI Codex CLI & ChatGPT Plus OAuth session tokens",
    "icon": "⚡",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "gpt-5.6-sol",
      "gpt-5.6-sol-review",
      "gpt-5.6-terra",
      "gpt-5.6-terra-review",
      "gpt-5.6-luna",
      "gpt-5.6-luna-review",
      "gpt-5.5",
      "gpt-5.5-review",
      "gpt-5.4",
      "gpt-5.4-review",
      "gpt-5.4-mini",
      "gpt-5.4-mini-review",
      "gpt-5.3-codex-spark",
      "gpt-5.3-codex-spark-review",
      "gpt-5.5-image"
    ],
    "alias": "cx"
  },
  {
    "id": "github",
    "name": "GitHub Copilot",
    "desc": "GitHub Copilot Device Flow authorization & token",
    "icon": "🐙",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "gpt-5.2",
      "gpt-5.2-codex",
      "gpt-5.3-codex",
      "gpt-5.4",
      "gpt-5.4-mini",
      "claude-haiku-4.5",
      "claude-opus-4.5",
      "claude-sonnet-4.5",
      "claude-sonnet-4.6",
      "claude-opus-4.6",
      "claude-opus-4.7",
      "gemini-2.5-pro",
      "gemini-3-flash-preview",
      "gemini-3.1-pro-preview",
      "grok-code-fast-1",
      "oswe-vscode-prime",
      "goldeneye-free-auto",
      "text-embedding-3-small",
      "text-embedding-3-large"
    ],
    "alias": "gh"
  },
  {
    "id": "claude",
    "name": "Claude Code (OAuth)",
    "desc": "Anthropic Console OAuth authorization code flow",
    "icon": "📜",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "claude-opus-5",
      "claude-fable-5",
      "claude-sonnet-5",
      "claude-haiku-4-5-20251001"
    ],
    "alias": "cc"
  },
  {
    "id": "kiro",
    "name": "Kiro IDE",
    "desc": "Kiro developer social/device authentication & AWS Cognito",
    "icon": "🔮",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "claude-opus-5",
      "claude-opus-5-thinking",
      "claude-opus-5-agentic",
      "claude-opus-5-thinking-agentic",
      "claude-opus-4.8",
      "claude-opus-4.8-thinking",
      "claude-opus-4.8-agentic",
      "claude-opus-4.8-thinking-agentic",
      "claude-opus-4.7",
      "claude-opus-4.7-thinking",
      "claude-opus-4.7-agentic",
      "claude-opus-4.7-thinking-agentic",
      "claude-opus-4.5",
      "claude-opus-4.5-thinking",
      "claude-opus-4.5-agentic",
      "claude-opus-4.5-thinking-agentic",
      "claude-sonnet-5",
      "claude-sonnet-4.5",
      "claude-haiku-4.5",
      "deepseek-3.2"
    ],
    "alias": "kr"
  },
  {
    "id": "qoder",
    "name": "Qoder IDE",
    "desc": "Qoder Developer Cloud auth token & device code",
    "icon": "⚡",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "ultimate",
      "auto",
      "performance",
      "efficient",
      "qmodel_preview",
      "qmodel_latest",
      "qmodel",
      "kmodel_latest",
      "kmodel",
      "gm51model",
      "dmodel",
      "dfmodel",
      "mmodel"
    ],
    "alias": "qd"
  },
  {
    "id": "cursor",
    "name": "Cursor IDE",
    "desc": "Cursor Workos token & local state.vscdb session import",
    "icon": "🖱️",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "default",
      "claude-4.5-opus-high-thinking",
      "claude-4.5-opus-high",
      "claude-4.5-sonnet-thinking",
      "claude-4.5-sonnet",
      "claude-4.5-haiku",
      "claude-4.5-opus",
      "gpt-5.2-codex",
      "claude-4.6-opus-max",
      "claude-4.6-sonnet-medium-thinking",
      "kimi-k2.5",
      "gemini-3-flash-preview",
      "gpt-5.2",
      "gpt-5.3-codex"
    ],
    "alias": "cu"
  },
  {
    "id": "windsurf",
    "name": "Windsurf IDE",
    "desc": "Codeium Windsurf auth token & session key",
    "icon": "🏄",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "swe-1.6-fast",
      "swe-1.6",
      "swe-1.5-fast",
      "swe-1.5",
      "claude-opus-4.7-max",
      "claude-opus-4.7-xhigh",
      "claude-opus-4.7-high",
      "claude-opus-4.7-medium",
      "claude-opus-4.7-low",
      "claude-opus-4.7-review",
      "claude-sonnet-4.6-thinking-1m",
      "claude-sonnet-4.6-1m",
      "claude-sonnet-4.6-thinking",
      "claude-sonnet-4.6",
      "claude-opus-4.6-thinking",
      "claude-opus-4.6",
      "claude-opus-4.5-thinking",
      "claude-opus-4.5",
      "claude-sonnet-4.5-thinking",
      "claude-sonnet-4.5",
      "claude-haiku-4.5",
      "gpt-5.5-xhigh-fast",
      "gpt-5.5-xhigh",
      "gpt-5.5-high-fast",
      "gpt-5.5-high",
      "gpt-5.5-medium-fast",
      "gpt-5.5-medium",
      "gpt-5.5-low-fast",
      "gpt-5.5-low",
      "gpt-5.5-none-fast",
      "gpt-5.5-none",
      "gpt-5.4-xhigh-fast",
      "gpt-5.4-xhigh",
      "gpt-5.4-high-fast",
      "gpt-5.4-high",
      "gpt-5.4-medium-fast",
      "gpt-5.4-medium",
      "gpt-5.4-low-fast",
      "gpt-5.4-low",
      "gpt-5.4-none-fast",
      "gpt-5.4-none",
      "gpt-5.4-mini-xhigh",
      "gpt-5.4-mini-high",
      "gpt-5.4-mini-medium",
      "gpt-5.4-mini-low",
      "gpt-5.3-codex-xhigh-fast",
      "gpt-5.3-codex-xhigh",
      "gpt-5.3-codex-high-fast",
      "gpt-5.3-codex-high",
      "gpt-5.3-codex-medium-fast",
      "gpt-5.3-codex-medium",
      "gpt-5.3-codex-low-fast",
      "gpt-5.3-codex-low",
      "gpt-5.2-xhigh",
      "gpt-5.2-high",
      "gpt-5.2-medium",
      "gpt-5.2-low",
      "gpt-5.2-none",
      "gpt-5",
      "gpt-4.1",
      "gpt-4.1-mini",
      "gpt-4.1-nano",
      "gpt-4o",
      "gpt-4o-mini",
      "gemini-3.1-pro-high",
      "gemini-3.1-pro-low",
      "gemini-3.0-flash-high",
      "gemini-3.0-flash-medium",
      "gemini-3.0-flash-low",
      "gemini-3.0-flash-minimal",
      "gemini-2.5-pro",
      "deepseek-v4",
      "kimi-k2.6",
      "kimi-k2.5",
      "glm-5.1"
    ],
    "alias": "ws"
  },
  {
    "id": "trae",
    "name": "Trae IDE",
    "desc": "ByteDance Trae Cloud-IDE-JWT authorization token",
    "icon": "🎯",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "auto",
      "work",
      "gemini-3.1-pro",
      "gemini-3-flash-solo",
      "minimax-m3",
      "minimax-m2.7",
      "kimi-k2.5",
      "gpt-5.4",
      "gpt-5.2"
    ],
    "alias": "tr"
  },
  {
    "id": "cline",
    "name": "Cline",
    "desc": "Cline VS Code extension OAuth session",
    "icon": "🤖",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "anthropic/claude-opus-4.7",
      "anthropic/claude-sonnet-4.6",
      "anthropic/claude-opus-4.6",
      "openai/gpt-5.3-codex",
      "openai/gpt-5.4",
      "google/gemini-3.1-pro-preview",
      "google/gemini-3.1-flash-lite-preview",
      "kwaipilot/kat-coder-pro"
    ],
    "alias": "cl"
  },
  {
    "id": "clinepass",
    "name": "ClinePass",
    "desc": "ClinePass subscription gateway integration",
    "icon": "🎟️",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "cline-pass/glm-5.2",
      "cline-pass/kimi-k2.7-code",
      "cline-pass/kimi-k2.6",
      "cline-pass/deepseek-v4-pro",
      "cline-pass/deepseek-v4-flash",
      "cline-pass/mimo-v2.5",
      "cline-pass/mimo-v2.5-pro",
      "cline-pass/minimax-m3",
      "cline-pass/qwen3.7-max",
      "cline-pass/qwen3.7-plus"
    ],
    "alias": "cp"
  },
  {
    "id": "codebuddy-cn",
    "name": "CodeBuddy (CN)",
    "desc": "Tencent CodeBuddy Chinese regional authentication",
    "icon": "🐧",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "glm-5.2",
      "glm-5.1",
      "glm-5.0",
      "glm-5.0-turbo",
      "glm-5v-turbo",
      "glm-4.7",
      "minimax-m3",
      "minimax-m2.7",
      "kimi-k2.7",
      "kimi-k2.6",
      "kimi-k2.5",
      "hy3-preview",
      "deepseek-v4-pro",
      "deepseek-v4-flash",
      "deepseek-v3-2-volc"
    ],
    "alias": "cd"
  },
  {
    "id": "codebuddy-intl",
    "name": "CodeBuddy (Intl)",
    "desc": "Tencent CodeBuddy International cloud token",
    "icon": "🌐",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "glm-5.2",
      "glm-5.1",
      "glm-5.0",
      "glm-5.0-turbo",
      "glm-5v-turbo",
      "glm-4.7",
      "minimax-m3",
      "minimax-m2.7",
      "kimi-k2.7",
      "kimi-k2.6",
      "kimi-k2.5",
      "hy3-preview",
      "deepseek-v4-pro",
      "deepseek-v4-flash",
      "deepseek-v3-2-volc"
    ],
    "alias": "cbai"
  },
  {
    "id": "devin-cli",
    "name": "Devin CLI",
    "desc": "Cognition Devin CLI session token import",
    "icon": "🧑‍💻",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "swe-1.6-fast",
      "swe-1.6",
      "swe-1.5-fast",
      "swe-1.5",
      "claude-opus-4.7-max",
      "claude-opus-4.7-high",
      "claude-opus-4.7-medium",
      "claude-opus-4.7-low",
      "claude-sonnet-4.6-thinking-1m",
      "claude-sonnet-4.6-thinking",
      "claude-sonnet-4.6",
      "claude-opus-4.6-thinking",
      "claude-opus-4.6",
      "claude-sonnet-4.5",
      "claude-haiku-4.5",
      "gpt-5.5-xhigh",
      "gpt-5.5-high",
      "gpt-5.5-medium",
      "gpt-5.5-low",
      "gpt-5.4-high",
      "gpt-5.4-medium",
      "gpt-5.4-low",
      "gpt-5.3-codex-high",
      "gpt-5.3-codex-medium",
      "gpt-5.3-codex-low",
      "gpt-5.2-high",
      "gpt-5.2-medium",
      "gpt-5.2-low",
      "gemini-3.1-pro-high",
      "gemini-3.1-pro-low",
      "gemini-3.0-flash-high",
      "gemini-2.5-pro",
      "deepseek-v4",
      "kimi-k2.6",
      "glm-5.1"
    ],
    "alias": "dv"
  },
  {
    "id": "kimi",
    "name": "Kimi Coding (OAuth)",
    "desc": "Moonshot Kimi developer OAuth authentication",
    "icon": "🌙",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [
      "kimi-k3",
      "k3",
      "kimi-for-coding",
      "kimi-for-coding-highspeed",
      "kimi-k2.7-code",
      "kimi-k2.7-code-highspeed",
      "kimi-k2.6",
      "kimi-k2.5",
      "kimi-k2.5-thinking",
      "kimi-latest"
    ],
    "alias": "km"
  },
  {
    "id": "zed",
    "name": "Zed Editor",
    "desc": "Zed assistant OAuth token & session key",
    "icon": "⚡",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [],
    "alias": "zd"
  },
  {
    "id": "gitlab",
    "name": "GitLab Duo",
    "desc": "GitLab Duo code suggestions personal token",
    "icon": "🦊",
    "category": "oauth",
    "authType": "oauth",
    "defaultModels": [],
    "alias": "gl"
  },
  {
    "id": "ollama",
    "name": "Ollama (Local)",
    "desc": "Local open-source models via HTTP (No key required)",
    "icon": "🦙",
    "category": "free",
    "authType": "noauth",
    "defaultUrl": "http://localhost:11434",
    "defaultModels": [
      "gpt-oss:120b",
      "kimi-k2.5",
      "glm-5",
      "minimax-m2.5",
      "glm-4.7-flash",
      "qwen3.5",
      "minimax-m3"
    ],
    "alias": "ollama"
  },
  {
    "id": "lmstudio",
    "name": "LM Studio",
    "desc": "Local LLM runner with OpenAI API endpoint",
    "icon": "💻",
    "category": "free",
    "authType": "noauth",
    "defaultUrl": "http://localhost:1234/v1",
    "defaultModels": [
      "local-model"
    ],
    "alias": "lmstudio"
  },
  {
    "id": "vllm",
    "name": "vLLM / SGLang",
    "desc": "High-throughput local LLM server",
    "icon": "⚡",
    "category": "free",
    "authType": "noauth",
    "defaultUrl": "http://localhost:8000/v1",
    "defaultModels": [
      "vllm-default"
    ],
    "alias": "vllm"
  },
  {
    "id": "opencode",
    "name": "OpenCode Zen (Free)",
    "desc": "Free public community AI endpoint (1-click activate)",
    "icon": "🎁",
    "category": "free",
    "authType": "free",
    "defaultUrl": "https://opencode.ai/zen/v1/chat/completions",
    "defaultModels": [
      "big-pickle",
      "mimo-v2.5-free",
      "ling-3.0-flash-fin-free",
      "nemotron-3-ultra-free",
      "nemotron-3.5-lightning-free",
      "laguna-s-2.1-free",
      "deepseek-v4-flash-free"
    ],
    "alias": "oc"
  },
  {
    "id": "opencode-go",
    "name": "OpenCode Go",
    "desc": "OpenCode Go fast paid inference",
    "icon": "🎁",
    "category": "apikey",
    "authType": "apikey",
    "defaultModels": [
      "glm-5.2",
      "glm-5.1",
      "kimi-k2.7-code",
      "kimi-k2.6",
      "deepseek-v4-pro",
      "deepseek-v4-flash",
      "mimo-v2.5",
      "mimo-v2.5-pro",
      "minimax-m3",
      "minimax-m2.7",
      "minimax-m2.5",
      "qwen3.7-max",
      "qwen3.7-plus",
      "qwen3.6-plus"
    ],
    "alias": "ocg"
  },
  {
    "id": "ddg",
    "name": "DuckDuckGo AI (Free)",
    "desc": "Free privacy-focused AI chat proxy",
    "icon": "🦆",
    "category": "free",
    "authType": "free",
    "defaultModels": [
      "gpt-4o-mini",
      "claude-3-haiku",
      "llama-3.1-70b",
      "mixtral-8x7b"
    ],
    "alias": "ddg"
  },
  {
    "id": "kilo-gateway",
    "name": "Kilo Gateway (Free)",
    "desc": "Kilo free tier public inference relay",
    "icon": "📦",
    "category": "free",
    "authType": "free",
    "defaultModels": [
      "kilo-auto/free",
      "nvidia/nemotron-3-super-120b-a12b:free",
      "nvidia/nemotron-3-ultra-550b-a55b:free",
      "kwaipilot/kat-coder-pro-v2.5:free",
      "kilo-auto/frontier",
      "kilo-auto/balanced"
    ],
    "alias": "kgw"
  },
  {
    "id": "mimo-free",
    "name": "Xiaomi MiMo (Free)",
    "desc": "Xiaomi MiMo developer free tier endpoints",
    "icon": "📱",
    "category": "free",
    "authType": "free",
    "defaultModels": [
      "mimo-auto"
    ],
    "alias": "mmf"
  },
  {
    "id": "bazaarlink",
    "name": "BazaarLink (Free)",
    "desc": "Decentralized AI gateway free nodes",
    "icon": "🏬",
    "category": "free",
    "authType": "free",
    "defaultModels": [
      "auto:free",
      "claude-opus-4.7",
      "claude-sonnet-4.6",
      "claude-haiku-4.5",
      "gpt-5.5",
      "gpt-5.4",
      "gpt-5.4-mini",
      "gpt-5.4-nano",
      "grok-4.3",
      "grok-4.20",
      "gemini-3.1-pro-preview",
      "gemini-3-flash-preview",
      "gemini-3.1-flash-lite-preview",
      "kimi-k2.6",
      "kimi-k2.5",
      "glm-5.1",
      "glm-5",
      "mimo-v2.5-pro",
      "mimo-v2.5",
      "minimax-m3",
      "minimax-m2.7",
      "minimax-m2.5",
      "qwen3.6-plus",
      "nemotron-3-super-120b-a12b"
    ],
    "alias": "bzl"
  },
  {
    "id": "edge-tts",
    "name": "Edge TTS (Free Voice)",
    "desc": "Microsoft Edge neural voice synthesis without credentials",
    "icon": "🗣️",
    "category": "free",
    "authType": "free",
    "defaultModels": [],
    "alias": "edge-tts"
  },
  {
    "id": "openai",
    "name": "OpenAI",
    "desc": "GPT-5.4, GPT-4o, o3-mini & embeddings",
    "icon": "⚡",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-proj-...",
    "defaultModels": [
      "gpt-5.4",
      "gpt-5.4-mini",
      "gpt-5.4-nano",
      "gpt-5.2",
      "gpt-5.1",
      "gpt-5",
      "gpt-5-mini",
      "gpt-5-nano",
      "gpt-4o",
      "gpt-4o-mini",
      "gpt-4-turbo",
      "gpt-4.1",
      "gpt-4.1-mini",
      "gpt-4.1-nano",
      "o3",
      "o3-mini",
      "o3-pro",
      "o4-mini",
      "o1",
      "o1-mini",
      "text-embedding-3-large",
      "text-embedding-3-small",
      "text-embedding-ada-002",
      "tts-1",
      "tts-1-hd",
      "gpt-4o-mini-tts",
      "whisper-1"
    ],
    "alias": "oa"
  },
  {
    "id": "anthropic",
    "name": "Anthropic",
    "desc": "Claude 3.7 Sonnet, Claude 3.5 Haiku, Opus",
    "icon": "🧠",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-ant-api03-...",
    "defaultModels": [
      "claude-sonnet-4-20250514",
      "claude-opus-4-20250514",
      "claude-3-5-sonnet-20241022"
    ],
    "alias": "ant"
  },
  {
    "id": "gemini",
    "name": "Google Gemini",
    "desc": "Gemini 3.6 Flash, 2.5 Pro, 2.5 Flash",
    "icon": "✨",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "AIzaSy...",
    "defaultModels": [
      "gemini-3.6-flash",
      "gemini-3.5-flash-lite",
      "gemini-3.1-pro-preview",
      "gemini-3.1-flash-lite-preview",
      "gemini-3-flash-preview",
      "gemini-2.5-pro",
      "gemini-2.5-flash",
      "gemini-2.5-flash-lite",
      "gemma-4-31b-it",
      "gemini-embedding-2-preview",
      "gemini-embedding-001",
      "text-embedding-005",
      "text-embedding-004",
      "gemini-3.1-flash-image-preview"
    ],
    "alias": "gemini"
  },
  {
    "id": "deepseek",
    "name": "DeepSeek",
    "desc": "DeepSeek-V4 Pro, DeepSeek-V3, Reasoner",
    "icon": "🐋",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-...",
    "defaultModels": [
      "deepseek-v4-pro",
      "deepseek-v4-pro-max",
      "deepseek-v4-pro-none",
      "deepseek-v4-flash",
      "deepseek-chat",
      "deepseek-reasoner"
    ],
    "alias": "ds"
  },
  {
    "id": "groq",
    "name": "Groq",
    "desc": "Ultra-fast LLaMA 3.3 70B & Mixtral",
    "icon": "🚀",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "gsk_...",
    "defaultModels": [
      "llama-3.3-70b-versatile",
      "meta-llama/llama-4-maverick-17b-128e-instruct",
      "qwen/qwen3-32b",
      "openai/gpt-oss-120b",
      "whisper-large-v3"
    ],
    "alias": "gq"
  },
  {
    "id": "openrouter",
    "name": "OpenRouter",
    "desc": "Aggregated AI models with unified key",
    "icon": "🌐",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-or-v1-...",
    "defaultModels": [
      "openai/text-embedding-3-large",
      "openai/text-embedding-3-small",
      "openai/text-embedding-ada-002",
      "qwen/qwen3-embedding-8b",
      "perplexity/pplx-embed-v1-4b",
      "perplexity/pplx-embed-v1-0.6b",
      "nvidia/llama-nemotron-embed-vl-1b-v2:free",
      "openai/gpt-4o-mini-tts",
      "openai/tts-1-hd",
      "openai/tts-1",
      "openai/dall-e-3"
    ],
    "alias": "or"
  },
  {
    "id": "mistral",
    "name": "Mistral AI",
    "desc": "Mistral Large, Codestral, Pixtral",
    "icon": "🌪️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "mistral-large-latest",
      "codestral-latest",
      "mistral-medium-latest",
      "mistral-embed"
    ],
    "alias": "mistral"
  },
  {
    "id": "together",
    "name": "Together AI",
    "desc": "Open source LLMs and fine-tunes",
    "icon": "🤝",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "meta-llama/Llama-3.3-70B-Instruct-Turbo",
      "deepseek-ai/DeepSeek-R1",
      "Qwen/Qwen3-235B-A22B",
      "meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8",
      "BAAI/bge-large-en-v1.5",
      "togethercomputer/m2-bert-80M-8k-retrieval"
    ],
    "alias": "tg"
  },
  {
    "id": "cerebras",
    "name": "Cerebras",
    "desc": "Wafer-scale LLaMA inference",
    "icon": "⚡",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "csk-...",
    "defaultModels": [
      "gpt-oss-120b",
      "zai-glm-4.7",
      "llama-3.3-70b",
      "llama-4-scout-17b-16e-instruct",
      "qwen-3-235b-a22b-instruct-2507",
      "qwen-3-32b"
    ],
    "alias": "cb"
  },
  {
    "id": "fireworks",
    "name": "Fireworks AI",
    "desc": "Fast serverless model endpoints",
    "icon": "🎆",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "fw_...",
    "defaultModels": [
      "accounts/fireworks/models/deepseek-v3p1",
      "accounts/fireworks/models/llama-v3p3-70b-instruct",
      "accounts/fireworks/models/qwen3-235b-a22b",
      "nomic-ai/nomic-embed-text-v1.5"
    ],
    "alias": "fw"
  },
  {
    "id": "xai",
    "name": "xAI (Grok)",
    "desc": "Grok-3, Grok-2 Mini models",
    "icon": "✖️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "xai-...",
    "defaultModels": [
      "grok-4",
      "grok-4-fast-reasoning",
      "grok-code-fast-1",
      "grok-3",
      "grok-2-image-1212"
    ],
    "alias": "xai"
  },
  {
    "id": "azure",
    "name": "Azure OpenAI",
    "desc": "Microsoft Azure OpenAI Service Endpoint",
    "icon": "☁️",
    "category": "apikey",
    "authType": "azure",
    "defaultModels": [],
    "alias": "az"
  },
  {
    "id": "cloudflare-ai",
    "name": "Cloudflare Workers AI",
    "desc": "Serverless GPU inference at the edge",
    "icon": "🔶",
    "category": "apikey",
    "authType": "cloudflare",
    "defaultModels": [
      "@cf/meta/llama-3.2-1b-instruct",
      "@cf/meta/llama-3.2-3b-instruct",
      "@cf/meta/llama-3.1-8b-instruct-fp8-fast",
      "@cf/meta/llama-3.1-8b-instruct-awq",
      "@cf/mistralai/mistral-small-3.1-24b-instruct",
      "@cf/meta/llama-3.1-70b-instruct-fp8-fast",
      "@cf/meta/llama-3.3-70b-instruct-fp8-fast",
      "@cf/deepseek-ai/deepseek-r1-distill-qwen-32b",
      "@cf/moonshotai/kimi-k2.5",
      "@cf/moonshotai/kimi-k2.6",
      "@cf/zai-org/glm-4.7-flash",
      "@cf/qwen/qwq-32b",
      "@cf/qwen/qwen2.5-coder-32b-instruct",
      "@cf/black-forest-labs/flux-2-klein-9b"
    ],
    "alias": "cf"
  },
  {
    "id": "perplexity",
    "name": "Perplexity AI",
    "desc": "Online search-augmented sonar models",
    "icon": "🔍",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "pplx-...",
    "defaultModels": [
      "sonar-pro",
      "sonar"
    ],
    "alias": "pplx"
  },
  {
    "id": "cohere",
    "name": "Cohere",
    "desc": "Command R+, Command R, & Cohere Embed",
    "icon": "🔮",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "command-r-plus-08-2024",
      "command-r-08-2024",
      "command-a-03-2025"
    ],
    "alias": "cohere"
  },
  {
    "id": "minimax",
    "name": "MiniMax",
    "desc": "MiniMax text-01, abab6.5, & voice synthesis",
    "icon": "⚡",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "MiniMax-M3",
      "MiniMax-M2.7",
      "MiniMax-M2.5",
      "MiniMax-M2.1",
      "minimax-image-01"
    ],
    "alias": "mm"
  },
  {
    "id": "qwen",
    "name": "Qwen (DashScope)",
    "desc": "Alibaba Cloud Qwen-Max, Qwen-Plus, Qwen-Coder",
    "icon": "☁️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-...",
    "defaultModels": [
      "qwen3-coder-plus",
      "qwen3-coder-flash",
      "vision-model",
      "coder-model"
    ],
    "alias": "qwen"
  },
  {
    "id": "baidu",
    "name": "Baidu Qianfan",
    "desc": "ERNIE-4.0 & Qianfan enterprise LLM models",
    "icon": "🇨🇳",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "deepseek-v4-pro",
      "deepseek-v4-flash",
      "glm-5.2",
      "glm-5.1",
      "kimi-k2.6",
      "qwen3.5-397b-a17b",
      "qwen3.5-27b"
    ],
    "alias": "qianfan"
  },
  {
    "id": "volcengine-ark",
    "name": "Volcengine Ark (Doubao)",
    "desc": "ByteDance Doubao enterprise model deployments",
    "icon": "🌋",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "Doubao-Seed-2.0-Code",
      "Doubao-Seed-2.0-pro",
      "Doubao-Seed-2.0-lite",
      "Doubao-Seed-Code",
      "DeepSeek-V4-Flash",
      "DeepSeek-V4-Pro",
      "GLM-5.1",
      "MiniMax-M2.7",
      "Kimi-K2.6"
    ],
    "alias": "ark"
  },
  {
    "id": "siliconflow",
    "name": "SiliconFlow",
    "desc": "High-speed cloud serverless inference endpoints",
    "icon": "🌊",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-...",
    "defaultModels": [
      "deepseek-ai/DeepSeek-V4-Pro",
      "deepseek-ai/DeepSeek-V4-Flash",
      "deepseek-ai/DeepSeek-V3.2",
      "deepseek-ai/DeepSeek-V3.2-Exp",
      "deepseek-ai/DeepSeek-V3.1",
      "deepseek-ai/DeepSeek-V3.1-Terminus",
      "deepseek-ai/DeepSeek-R1",
      "Qwen/Qwen3.5-397B-A17B",
      "Qwen/Qwen3.5-122B-A10B",
      "zai-org/GLM-5.1",
      "zai-org/GLM-5",
      "moonshotai/Kimi-K2.6",
      "moonshotai/Kimi-K2.5",
      "openai/gpt-oss-120b",
      "MiniMaxAI/MiniMax-M2.5",
      "inclusionAI/Ling-flash-2.0"
    ],
    "alias": "siliconflow"
  },
  {
    "id": "sambanova",
    "name": "SambaNova",
    "desc": "DataScale ultra-low latency inference engine",
    "icon": "⚡",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "MiniMax-M2.7"
    ],
    "alias": "samba"
  },
  {
    "id": "hyperbolic",
    "name": "Hyperbolic",
    "desc": "Decentralized GPU compute inference cloud",
    "icon": "📐",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "Qwen/QwQ-32B",
      "deepseek-ai/DeepSeek-R1",
      "deepseek-ai/DeepSeek-V3",
      "meta-llama/Llama-3.3-70B-Instruct",
      "meta-llama/Llama-3.2-3B-Instruct",
      "Qwen/Qwen2.5-72B-Instruct",
      "Qwen/Qwen2.5-Coder-32B-Instruct",
      "NousResearch/Hermes-3-Llama-3.1-70B"
    ],
    "alias": "hyp"
  },
  {
    "id": "voyage-ai",
    "name": "Voyage AI",
    "desc": "State-of-the-art embedding and reranker models",
    "icon": "🧭",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "pa-...",
    "defaultModels": [
      "voyage-3-large",
      "voyage-3.5",
      "voyage-3.5-lite",
      "voyage-code-3",
      "voyage-finance-2",
      "voyage-law-2",
      "voyage-multilingual-2"
    ],
    "alias": "voyage-ai"
  },
  {
    "id": "exa",
    "name": "Exa AI Search",
    "desc": "Neural web search and content retrieval API",
    "icon": "🔎",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [],
    "alias": "exa"
  },
  {
    "id": "tavily",
    "name": "Tavily Search",
    "desc": "Search engine optimized for LLM agents & RAG",
    "icon": "🌐",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "tvly-...",
    "defaultModels": [],
    "alias": "tavily"
  },
  {
    "id": "jina-ai",
    "name": "Jina AI",
    "desc": "Jina Reader, embeddings, and web scraping API",
    "icon": "🦊",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "jina_...",
    "defaultModels": [
      "jina-embeddings-v3",
      "jina-embeddings-v2-base-en",
      "jina-embeddings-v2-base-code"
    ],
    "alias": "jina"
  },
  {
    "id": "elevenlabs",
    "name": "ElevenLabs",
    "desc": "Realistic AI voice cloning and speech synthesis",
    "icon": "🎙️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk_...",
    "defaultModels": [
      "eleven_multilingual_v2",
      "eleven_turbo_v2_5"
    ],
    "alias": "el"
  },
  {
    "id": "deepgram",
    "name": "DeepGram STT",
    "desc": "Real-time speech-to-text audio transcription",
    "icon": "🎙️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "nova-3"
    ],
    "alias": "dg"
  },
  {
    "id": "cartesia",
    "name": "Cartesia TTS",
    "desc": "Sonic ultra-low-latency real-time voice API",
    "icon": "🔊",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "sonic-2",
      "sonic-3"
    ],
    "alias": "cartesia"
  },
  {
    "id": "fal-ai",
    "name": "Fal AI Media",
    "desc": "Flux, SDXL, Fast Lightning generation endpoints",
    "icon": "🎨",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "...",
    "defaultModels": [
      "fal-ai/flux/schnell"
    ],
    "alias": "fal"
  },
  {
    "id": "stability-ai",
    "name": "Stability AI",
    "desc": "Stable Diffusion 3.5, Ultra, and Image API",
    "icon": "🖼️",
    "category": "apikey",
    "authType": "apikey",
    "keyPlaceholder": "sk-...",
    "defaultModels": [
      "stable-image-ultra"
    ],
    "alias": "stability"
  },
  {
    "id": "iflow",
    "name": "iFlow (Web Session)",
    "desc": "Browser session cookie authentication",
    "icon": "🍪",
    "category": "apikey",
    "authType": "cookie",
    "defaultModels": [
      "qwen3-coder-plus",
      "qwen3-max",
      "qwen3-vl-plus",
      "qwen3-max-preview",
      "qwen3-235b",
      "qwen3-235b-a22b-instruct",
      "qwen3-235b-a22b-thinking-2507",
      "qwen3-32b",
      "kimi-k2",
      "deepseek-v3.2",
      "deepseek-v3.1",
      "deepseek-v3",
      "deepseek-r1",
      "glm-4.7",
      "iflow-rome-30ba3b"
    ],
    "alias": "if"
  },
  {
    "id": "grok-web",
    "name": "Grok Web (Cookie)",
    "desc": "Grok browser SSO cookie authentication",
    "icon": "🍪",
    "category": "apikey",
    "authType": "cookie",
    "defaultModels": [
      "grok-3",
      "grok-3-mini",
      "grok-3-thinking",
      "grok-4",
      "grok-4-mini",
      "grok-4-thinking",
      "grok-4-heavy",
      "grok-4.1-mini",
      "grok-4.1-fast",
      "grok-4.1-expert",
      "grok-4.1-thinking",
      "grok-4.2"
    ],
    "alias": "gw"
  }
];

function renderProviderIcon(providerId, fallbackEmoji = '🔌') {
  let iconName = (providerId || '').toLowerCase();
  const aliasMap = {
    'custom-openai-compatible': 'openai',
    'custom-anthropic-compatible': 'anthropic',
    'custom-embedding': 'openai',
    'ollama-local': 'ollama',
    'lmstudio': 'local-device',
    'vllm': 'local-device',
    'opencode': 'opencode',
    'duckduckgo-ai': 'brave-search',
    'xiaomi-mimo': 'xiaomi-mimo',
    'baidu-qianfan': 'baidu',
    'cloudflare': 'cloudflare-ai',
    'jina': 'jina-ai',
    'codebuddy': 'codebuddy-cn',
    'devin': 'devin-cli'
  };
  if (aliasMap[iconName]) {
    iconName = aliasMap[iconName];
  }
  if (iconName.startsWith('openai-compatible')) iconName = 'openai';
  if (iconName.startsWith('anthropic-compatible')) iconName = 'anthropic';
  const iconSrc = `/providers/${encodeURIComponent(iconName)}.png`;
  return `
    <div class="provider-brand-icon-wrapper">
      <img src="${iconSrc}" class="provider-brand-icon" alt="${escapeHtml(providerId)}" onerror="this.onerror=null; this.style.display='none'; this.nextElementSibling.style.display='inline-block';" />
      <span class="fallback-emoji" style="display:none; font-size: 13px; line-height: 1;">${fallbackEmoji}</span>
    </div>
  `;
}

function getProviderStats(providerId, connections = []) {
  const items = connections.filter((c) => (c.provider || '').toLowerCase() === providerId.toLowerCase());
  const active = items.filter(isItemActive);
  return { total: items.length, active: active.length, items };
}

// ─────────────────────────────────────────────────────────────
// 1. MAIN PROVIDERS CATALOG VIEW (#providers)
// ─────────────────────────────────────────────────────────────
function renderProviders(payload) {
  const conns = payload.connections || [];
  const nodes = payload.nodes || [];
  
  const openaiNodes = nodes.filter((n) => n.type === 'openai-compatible');
  const anthropicNodes = nodes.filter((n) => n.type === 'anthropic-compatible');

  const oauthItems = KNOWN_PROVIDER_CATALOG.filter((p) => p.category === 'oauth');
  const freeItems = KNOWN_PROVIDER_CATALOG.filter((p) => p.category === 'free');
  const apiKeyItems = KNOWN_PROVIDER_CATALOG.filter((p) => p.category === 'apikey');

  function renderCategoryGrid(items, categoryTitle, extraActionHtml = '') {
    return `
      <section class="provider-category-section">
        <div class="provider-category-head">
          <h3>${escapeHtml(categoryTitle)}</h3>
          ${extraActionHtml}
        </div>
        <div class="category-card-grid">
          ${items.map((cat) => {
            const stats = getProviderStats(cat.id, conns);
            const hasConns = stats.total > 0;
            return `
              <div class="provider-cat-card" data-open-provider="${escapeHtml(cat.id)}">
                <div class="provider-cat-main">
                  ${renderProviderIcon(cat.id, cat.icon)}
                  <div class="provider-cat-info">
                    <strong>${escapeHtml(cat.name)}</strong>
                    <p>${escapeHtml(cat.desc)}</p>
                  </div>
                </div>
                <div class="provider-cat-meta">
                  <span class="provider-cat-badge ${hasConns ? 'has-conns' : 'no-conns'}">
                    ${hasConns ? `${stats.active}/${stats.total} Connected` : 'Not Configured'}
                  </span>
                  <span class="table-badge" style="font-size:8px; padding:2px 5px; background:rgba(200,255,99,0.08); border:1px solid rgba(200,255,99,0.25); color:var(--lime);">
                    ${escapeHtml(cat.alias || cat.id)}/
                  </span>
                </div>
              </div>
            `;
          }).join('')}
        </div>
      </section>
    `;
  }

  function renderCustomNodesSection() {
    const allCustomNodes = [...openaiNodes, ...anthropicNodes];
    return `
      <section class="provider-category-section">
        <div class="provider-category-head">
          <div>
            <h3>Custom Providers (OpenAI &amp; Anthropic Compatible)</h3>
            <p style="font-size:11px; color:var(--muted); margin:2px 0 0;">Create custom AI endpoints, local model servers, or vLLM clusters with custom routing prefixes.</p>
          </div>
          <div class="group-actions" style="display:flex; gap:8px;">
            <button class="solid-button" id="btn-add-openai-node" type="button" style="font-size:11px; padding:6px 12px;">+ Add OpenAI Compatible</button>
            <button class="secondary-button" id="btn-add-anthropic-node" type="button" style="font-size:11px; padding:6px 12px;">+ Add Anthropic Compatible</button>
          </div>
        </div>

        ${allCustomNodes.length === 0 ? `
          <div class="card" style="padding:28px 20px; text-align:center; border:1px dashed var(--line); border-radius:10px; background:rgba(255,255,255,0.01);">
            <span class="material-symbols-outlined" style="font-size:32px; color:var(--muted); opacity:0.6; margin-bottom:8px;">extension</span>
            <p style="font-size:12.5px; color:var(--muted); margin:0 0 10px;">No custom compatible nodes created yet.</p>
            <p style="font-size:11px; color:#5a6e82; margin:0;">Click the buttons above to add an OpenAI or Anthropic compatible endpoint with its custom prefix and base URL.</p>
          </div>
        ` : `
          <div class="category-card-grid">
            ${allCustomNodes.map((node) => {
              const isAnthropic = node.type === 'anthropic-compatible';
              const prefix = node.prefix || node.name || 'custom';
              const stats = getProviderStats(node.id, conns);
              const hasConns = stats.total > 0;
              return `
                <div class="provider-cat-card" data-open-provider="${escapeHtml(node.id)}">
                  <div class="provider-cat-main">
                    <div class="provider-brand-icon-wrapper" style="background:${isAnthropic ? 'rgba(217,119,87,0.12)' : 'rgba(16,163,127,0.12)'}; border-color:${isAnthropic ? '#d97757' : '#10a37f'};">
                      <span style="font-size:11px; font-weight:700; font-family:var(--mono); color:${isAnthropic ? '#d97757' : '#10a37f'};">${isAnthropic ? 'AC' : 'OC'}</span>
                    </div>
                    <div class="provider-cat-info">
                      <strong>${escapeHtml(node.name || (isAnthropic ? 'Anthropic Compatible' : 'OpenAI Compatible'))}</strong>
                      <p style="font-family:var(--mono); font-size:10px; color:#6b7c8e;">${escapeHtml(node.baseUrl || '')}</p>
                    </div>
                  </div>
                  <div class="provider-cat-meta">
                    <span class="provider-cat-badge ${hasConns ? 'has-conns' : 'no-conns'}">
                      ${hasConns ? `${stats.active}/${stats.total} Keys` : 'No API Key Added'}
                    </span>
                    <span class="table-badge" style="font-size:8px; padding:2px 5px; background:rgba(200,255,99,0.08); border:1px solid rgba(200,255,99,0.25); color:var(--lime);">
                      ${escapeHtml(prefix)}/
                    </span>
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        `}
      </section>
    `;
  }

  return `
    <div class="providers-view-container">
      <div class="providers-top-bar card">
        <div>
          <span class="kicker">ROUTING FABRIC / PROVIDER NODES</span>
          <h2>Provider Catalog &amp; Nodes</h2>
          <p>Organized by category. Click any provider or node to manage API keys, routing prefix, and custom models.</p>
        </div>
        <div class="top-actions">
          <button class="solid-button" id="btn-open-add-provider"><span>+</span> Connect Provider Account</button>
        </div>
      </div>

      <!-- 1. Custom Compatible Endpoints (Node Types) -->
      ${renderCustomNodesSection()}

      <!-- 2. OAuth & Device Flow Providers -->
      ${renderCategoryGrid(oauthItems, 'OAuth & Device Flow Providers')}

      <!-- 3. Free & Local Providers -->
      ${renderCategoryGrid(freeItems, 'Free & Local Providers')}
      <!-- 4. Standard API Key Providers -->
      ${renderCategoryGrid(apiKeyItems, 'API Key Providers')}
    </div>
  `;
}

// ─────────────────────────────────────────────────────────────
// 2. PROVIDER DETAIL VIEW (#provider/<id>)
// ─────────────────────────────────────────────────────────────
async function renderProviderDetail(provId) {
  content.innerHTML = '<div class="card generic-empty"><span class="loading-line"></span><p>Loading provider node details...</p></div>';
  
  try {
    const [connPayload, modelPayload, poolPayload, settingsPayload, customPayload, prefixPayload, nodesPayload] = await Promise.all([
      request('/api/providers').catch(() => ({ connections: [] })),
      request('/models').catch(() => ({ data: [] })),
      request('/api/proxy-pools').catch(() => ({ proxyPools: [] })),
      request('/api/settings').catch(() => ({})),
      request('/api/custom-models').catch(() => ({ customModels: [] })),
      request('/api/provider-prefixes').catch(() => ({ prefixes: {} })),
      request('/api/provider-nodes').catch(() => ({ nodes: [] }))
    ]);

    const allConns = connPayload.connections || [];
    const conns = allConns.filter((c) => (c.provider || '').toLowerCase() === provId.toLowerCase());
    const proxyPools = poolPayload.proxyPools || [];
    const customNodes = nodesPayload.nodes || [];
    const matchedNode = customNodes.find((n) => n.id === provId);

    let meta = KNOWN_PROVIDER_CATALOG.find((p) => p.id === provId);
    if (!meta && matchedNode) {
      const isAnthropic = matchedNode.type === 'anthropic-compatible';
      meta = {
        id: matchedNode.id,
        name: matchedNode.name || (isAnthropic ? 'Anthropic Compatible' : 'OpenAI Compatible'),
        desc: matchedNode.baseUrl || 'Custom Runtime Endpoint',
        icon: isAnthropic ? '🎭' : '🔌',
        category: 'custom',
        authType: 'apikey',
        alias: matchedNode.prefix || 'custom',
        defaultModels: []
      };
    }
    if (!meta) {
      meta = { id: provId, name: provId.toUpperCase(), desc: 'Custom Provider', icon: '🔌', defaultModels: [] };
    }

    const isFreeProvider = meta.category === 'free' || meta.authType === 'free' || meta.authType === 'noauth';
    const activePrefix = (prefixPayload.prefixes || {})[provId] || matchedNode?.prefix || meta.alias || provId;
    const providerStrategy = (settingsPayload.providerStrategies || {})[provId] || {};
    let freeBoundPoolId = providerStrategy.proxyPoolId || '__none__';
    if (freeBoundPoolId === '__none__' && conns[0]) {
      try {
        const cd = typeof conns[0].data === 'string' ? JSON.parse(conns[0].data) : (conns[0].data || {});
        if (cd.proxyPoolId) freeBoundPoolId = cd.proxyPoolId;
      } catch {}
    }
    
    // Extract unique clean models for this provider (preserve internal slashes for sub-path models like f/mimo-v2.5-free)
    const modelSet = new Set();
    const customModelIdsSet = new Set();

    (meta.defaultModels || []).forEach((m) => {
      let rawId = String(m).trim();
      if (rawId.startsWith(`${activePrefix}/`)) rawId = rawId.slice(activePrefix.length + 1);
      if (rawId) modelSet.add(rawId);
    });

    conns.forEach((c) => {
      let d = {};
      try { d = typeof c.data === 'string' ? JSON.parse(c.data) : (c.data || {}); } catch {}
      if (d.defaultModel) {
        let rawId = String(d.defaultModel).trim();
        if (rawId.startsWith(`${activePrefix}/`)) rawId = rawId.slice(activePrefix.length + 1);
        if (rawId) modelSet.add(rawId);
      }
      if (Array.isArray(d.customModels)) {
        d.customModels.forEach((cm) => {
          let rawId = String(cm).trim();
          if (rawId.startsWith(`${activePrefix}/`)) rawId = rawId.slice(activePrefix.length + 1);
          if (rawId) modelSet.add(rawId);
        });
      }
    });

    const provAliases = new Set([
      provId.toLowerCase(),
      (meta.alias || '').toLowerCase(),
      (activePrefix || '').toLowerCase(),
      (meta.id || '').toLowerCase()
    ]);
    if (matchedNode?.prefix) provAliases.add(matchedNode.prefix.toLowerCase());
    if (provId.toLowerCase() === 'antigravity') provAliases.add('ag');
    if (provId.toLowerCase() === 'codex') provAliases.add('cx');
    if (provId.toLowerCase() === 'github' || provId.toLowerCase() === 'copilot') {
      provAliases.add('github');
      provAliases.add('copilot');
      provAliases.add('gh');
    }
    if (provId.toLowerCase() === 'qoder') provAliases.add('qd');
    if (provId.toLowerCase() === 'grok-cli') provAliases.add('gcli');
    if (provId.toLowerCase() === 'opencode') { provAliases.add('opencode-go'); }

    // Custom models registered in DB
    (customPayload.customModels || []).forEach((cm) => {
      const p = (cm.providerAlias || cm.provider || '').toLowerCase();
      if (provAliases.has(p)) {
        let rawId = String(cm.id).trim();
        if (rawId.startsWith(`${activePrefix}/`)) rawId = rawId.slice(activePrefix.length + 1);
        if (rawId) {
          modelSet.add(rawId);
          customModelIdsSet.add(rawId);
        }
      }
    });

    const modelsList = Array.from(modelSet);
    const accountPageSize = 10;
    const accountPageCount = Math.max(1, Math.ceil(conns.length / accountPageSize));
    const accountPage = Math.min(providerAccountPages.get(provId) || 1, accountPageCount);
    providerAccountPages.set(provId, accountPage);
    const accountOffset = (accountPage - 1) * accountPageSize;
    const visibleConns = conns.slice(accountOffset, accountOffset + accountPageSize);
    const providerProxyMode = providerStrategy.rotateStrategy && providerStrategy.rotateStrategy !== 'none'
      ? providerStrategy.rotateStrategy
      : (providerStrategy.proxyPoolId ? 'fixed' : 'direct');
    content.innerHTML = `
      <div class="provider-detail-container">
        <div class="provider-detail-header card">
          <div class="detail-head-left">
            <button class="back-btn" id="btn-back-to-providers">&larr; Back to Catalog</button>
            ${renderProviderIcon(meta.id, meta.icon)}
            <div>
              <span class="kicker">PROVIDER / ${escapeHtml(meta.category?.toUpperCase() || 'NODE')}</span>
              <h2>${escapeHtml(meta.name)}</h2>
              <p style="margin:2px 0 0;">${escapeHtml(meta.desc)}</p>
              <div style="display:flex; align-items:center; gap:8px; margin-top:6px;">
                <span class="kicker" style="font-size:8.5px; color:var(--muted);">ROUTING PREFIX:</span>
                <code class="model-id-code" style="color:var(--lime); font-size:11px; font-weight:600;">${escapeHtml(activePrefix)}/</code>
                <button class="secondary-button" id="btn-edit-provider-prefix" type="button" style="font-size:9.5px; padding:2px 7px;" title="Change routing prefix">
                  ✏️ Edit Prefix
                </button>
                ${(prefixPayload.prefixes || {})[provId] ? `
                  <button class="cancel-button" id="btn-reset-provider-prefix" type="button" style="font-size:9.5px; padding:2px 6px;" title="Reset prefix to default">
                    Reset
                  </button>
                ` : ''}
              </div>
            </div>
          </div>
          <div class="top-actions" style="display:flex; align-items:center; gap:8px;">
            ${matchedNode ? `
              <button class="danger-button" id="btn-delete-provider-node" type="button" style="font-size:11px; padding:6px 12px;" title="Delete this custom provider node and all its accounts">
                Delete Node
              </button>
            ` : ''}
            <button class="secondary-button" data-reset-health="${escapeHtml(provId)}">Reset Health</button>
            ${!isFreeProvider ? `<button class="solid-button" data-add-account="${escapeHtml(provId)}"><span>+</span> Add Account</button>` : ''}
          </div>
        </div>

        <div class="detail-grid-layout">
          <!-- 1. Connections / Free Proxy Router Card -->
          ${isFreeProvider ? `
            <div class="detail-panel card">
              <div class="detail-panel-head">
                <div>
                  <h3>Outbound Proxy &amp; Relay Routing</h3>
                  <span class="table-badge active" style="font-size:8px;">FREE &bull; NO API KEY REQUIRED</span>
                </div>
              </div>

              <div style="display:grid; gap:10px; padding:4px 0;">
                <p style="font-size:11.5px; color:var(--text); margin:0; line-height:1.4;">
                  <strong>${escapeHtml(meta.name)}</strong> operates without personal API keys. Optionally route requests through a proxy pool or enable smart rotation to bypass IP-based rate limits.
                </p>

                <div style="display:grid; gap:10px; background:#080b10; border:1px solid var(--line); border-radius:6px; padding:12px;">
                  <!-- 1. Single Pool Selector -->
                  <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
                    Proxy Pool Selection (Single / Fixed)
                    <select id="free-node-proxy-select" style="background:#05070a; border:1px solid var(--line); border-radius:5px; padding:7px 10px; font:11px var(--mono); color:var(--text);" ${(providerStrategy.rotateStrategy || 'none') !== 'none' ? 'disabled style="opacity:0.45; cursor:not-allowed;"' : ''}>
                      <option value="__none__" ${freeBoundPoolId === '__none__' ? 'selected' : ''}>Direct (No Proxy)</option>
                      ${proxyPools.map((p) => {
                        const displayName = p.name || p.proxyUrl || p.id;
                        const pType = (p.type || 'http').toUpperCase();
                        return `<option value="${escapeHtml(p.id)}" ${p.id === freeBoundPoolId ? 'selected' : ''}>${escapeHtml(displayName)} (${pType})</option>`;
                      }).join('')}
                    </select>
                  </label>

                  <!-- 2. Smart Rotation Strategy Selector -->
                  <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
                    Rotation Strategy (Anti-Rate Limit)
                    <select id="free-node-rotate-select" style="background:#05070a; border:1px solid var(--line); border-radius:5px; padding:7px 10px; font:11px var(--mono); color:var(--text);">
                      <option value="none" ${(providerStrategy.rotateStrategy || 'none') === 'none' ? 'selected' : ''}>None (Use single proxy pool selected above)</option>
                      <option value="round-robin" ${providerStrategy.rotateStrategy === 'round-robin' ? 'selected' : ''} ${proxyPools.filter(isItemActive).length < 2 ? 'disabled' : ''}>Round-Robin (Rotate across all active pools)${proxyPools.filter(isItemActive).length < 2 ? ' — (Need 2+ active pools)' : ''}</option>
                      <option value="random" ${providerStrategy.rotateStrategy === 'random' ? 'selected' : ''} ${proxyPools.filter(isItemActive).length < 2 ? 'disabled' : ''}>Random (Pick random pool per request)${proxyPools.filter(isItemActive).length < 2 ? ' — (Need 2+ active pools)' : ''}</option>
                    </select>
                  </label>

                  <!-- 3. Dynamic Explanation Note -->
                  <div id="free-proxy-explanation-note" style="font-size:11px; color:#9db2c6; line-height:1.4; padding:7px 10px; background:#05070b; border:1px dashed var(--line); border-radius:4px;">
                    ${(providerStrategy.rotateStrategy || 'none') === 'none' 
                      ? (freeBoundPoolId === '__none__' 
                          ? '🔗 Requests connect directly from your machine\'s local IP.' 
                          : '🔗 Requests route through the single fixed proxy pool selected above.')
                      : `🔄 Dynamic rotation is ACTIVE: Requests cycle across all active proxy pools. The single pool selector above is bypassed.`}
                  </div>

                  <div style="display:flex; justify-content:space-between; align-items:center; margin-top:4px;">
                    <button class="solid-button" id="btn-save-free-proxy" type="button">
                      Save Proxy Settings
                    </button>
                    <span id="free-proxy-save-status" style="font-size:10px; font-family:var(--mono); color:var(--lime);"></span>
                  </div>
                </div>
              </div>
            </div>
          ` : `
            <div class="detail-panel card">
              <div class="detail-panel-head" style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px;">
                <div>
                  <h3>Connected Accounts</h3>
                  <span class="kicker">${conns.filter(isItemActive).length} / ${conns.length} Active</span>
                </div>
                <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
                  <!-- Per-Provider Strategy (100% 9router Parity) -->
                  <div style="display:inline-flex; align-items:center; gap:6px; background:#080b10; border:1px solid var(--line); border-radius:5px; padding:3px 8px;">
                    <span style="font-size:9.5px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">Strategy:</span>
                    <select id="prov-routing-strategy-select" style="background:#05070a; border:1px solid var(--line-subtle); color:var(--text-bright); font:10px var(--mono); padding:2px 4px; border-radius:3px;">
                      <option value="fallback" ${(!providerStrategy.fallbackStrategy || providerStrategy.fallbackStrategy === 'fallback') ? 'selected' : ''}>Fallback (Priority Order)</option>
                      <option value="round-robin" ${providerStrategy.fallbackStrategy === 'round-robin' ? 'selected' : ''}>Round-Robin (Balance Accounts)</option>
                    </select>
                    <span id="prov-sticky-wrapper" style="display:${providerStrategy.fallbackStrategy === 'round-robin' ? 'inline-flex' : 'none'}; align-items:center; gap:3px;">
                      <span style="font-size:9px; color:var(--dim); font-family:var(--mono);">Sticky:</span>
                      <input type="number" id="prov-sticky-input" min="1" max="100" value="${providerStrategy.stickyRoundRobinLimit || 1}" style="width:36px; background:#05070a; border:1px solid var(--line-subtle); color:var(--text); font:10px var(--mono); padding:2px 4px; border-radius:3px; text-align:center;" title="Sticky consecutive requests before rotating" />
                    </span>
                  </div>
                  <button class="secondary-button" data-add-account="${escapeHtml(provId)}">+ Add</button>
                </div>
              </div>
              ${conns.length > 1 ? `
                <div class="bulk-proxy-bar" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:8px 10px; display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; margin-bottom:8px;">
                  <div style="display:flex; align-items:center; gap:6px;">
                    <span class="kicker" style="font-size:8px;">BULK PROXY:</span>
                    <select id="bulk-apply-proxy-select" style="font-size:10px; padding:3px 6px; background:#05070a; border:1px solid var(--line); color:var(--text); border-radius:4px;">
                      <option value="__none__">Direct (No Proxy)</option>
                      ${proxyPools.map((p) => {
                        const displayName = p.name || p.proxyUrl || p.id;
                        const pType = (p.type || 'http').toUpperCase();
                        return `<option value="${escapeHtml(p.id)}">${escapeHtml(displayName)} (${pType})</option>`;
                      }).join('')}
                    </select>
                    <button type="button" class="secondary-button" id="btn-apply-bulk-proxy" style="font-size:9.5px; padding:3px 7px;">Apply to All ${conns.length} Accounts</button>
                  </div>
                  <div style="display:flex; align-items:center; gap:6px;">
                    ${proxyPools.filter(isItemActive).length >= 2 ? `
                      <button type="button" class="secondary-button" id="btn-distribute-proxies" style="font-size:9.5px; padding:3px 7px;" title="Distribute accounts across active pools evenly">Distribute 1:1</button>
                    ` : ''}
                    <button type="button" class="secondary-button" id="btn-reset-all-proxies" style="font-size:9.5px; padding:3px 7px;">Reset All to Direct</button>
                  </div>
                </div>
              ` : ''}

              <div class="bulk-proxy-bar" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:8px 10px; display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:8px;">
                <span class="kicker" style="font-size:8px;">PROVIDER PROXY:</span>
                <select id="provider-proxy-mode" style="font-size:10px; padding:3px 6px; background:#05070a; border:1px solid var(--line); color:var(--text); border-radius:4px;" title="Provider proxy overrides account assignments unless set to Direct">
                  <option value="direct" ${providerProxyMode === 'direct' ? 'selected' : ''}>Direct (No Proxy)</option>
                  <option value="fixed" ${providerProxyMode === 'fixed' ? 'selected' : ''}>One Fixed Proxy Pool</option>
                  <option value="round-robin" ${providerProxyMode === 'round-robin' ? 'selected' : ''}>Smart Round-Robin (All Active Pools)</option>
                  <option value="random" ${providerProxyMode === 'random' ? 'selected' : ''}>Smart Random (All Active Pools)</option>
                </select>
                <select id="provider-proxy-pool" style="font-size:10px; padding:3px 6px; background:#05070a; border:1px solid var(--line); color:var(--text); border-radius:4px; ${providerProxyMode === 'fixed' ? '' : 'display:none;'}" title="Fixed proxy pool for this provider">
                  <option value="__none__">Select pool...</option>
                  ${proxyPools.filter(isItemActive).map((p) => {
                    const displayName = p.name || p.proxyUrl || p.id;
                    return `<option value="${escapeHtml(p.id)}" ${p.id === providerStrategy.proxyPoolId ? 'selected' : ''}>${escapeHtml(displayName)} (${escapeHtml((p.type || 'http').toUpperCase())})</option>`;
                  }).join('')}
                </select>
                <button type="button" class="secondary-button" id="btn-save-provider-proxy" style="font-size:9.5px; padding:3px 7px;">Save Provider Proxy</button>
                <small style="font-size:9px; color:var(--dim);">Provider setting takes precedence; Direct falls back to the account assignment.</small>
              </div>

              <div class="conn-rows-list">
                ${conns.length === 0 ? `
                  <div class="card generic-empty" style="padding: 24px;">
                    <p>No accounts connected for this provider.</p>
                    <button class="solid-button" data-add-account="${escapeHtml(provId)}" style="margin-top: 10px;">+ Add First Account</button>
                  </div>
                ` : visibleConns.map((conn, idx) => {
                  const absoluteIdx = accountOffset + idx;
                  let parsed = {};
                  try { parsed = typeof conn.data === 'string' ? JSON.parse(conn.data) : (conn.data || {}); } catch {}
                  const hint = parsed.apiKey ? maskKey(parsed.apiKey) : (parsed.baseUrl || conn.email || conn.authType || '--');
                  const boundPoolId = parsed.proxyPoolId || '__none__';

                  return `
                    <div class="detail-conn-row ${conn.isActive === 1 ? '' : 'inactive-card'}">
                      <div class="conn-left-side">
                        <div class="reorder-btns">
                          <button class="reorder-btn" data-swap-up="${absoluteIdx}" ${absoluteIdx === 0 ? 'disabled style="opacity:0.2"' : ''} title="Move Up">&blacktriangle;</button>
                          <button class="reorder-btn" data-swap-down="${absoluteIdx}" ${absoluteIdx === conns.length - 1 ? 'disabled style="opacity:0.2"' : ''} title="Move Down">&blacktriangledown;</button>
                        </div>
                        <div class="conn-main-info">
                          <strong>${escapeHtml(conn.name || conn.id)}</strong>
                          <small>${escapeHtml(hint)} &bull; Priority: #${escapeHtml(conn.priority ?? idx + 1)}</small>
                        </div>
                      </div>

                      <div class="conn-right-actions">
                        <!-- Proxy Pool Selector -->
                        <select class="proxy-select" data-conn-proxy="${escapeHtml(conn.id)}" style="font-size:10px; padding:3px 6px; background:#080b10; border:1px solid var(--line); color:var(--muted); border-radius:4px;" title="Assign Outbound Proxy Pool">
                          <option value="__none__" ${boundPoolId === '__none__' ? 'selected' : ''}>Direct (No Proxy)</option>
                          ${proxyPools.map((p) => {
                            const displayName = p.name || p.proxyUrl || p.id;
                            const pType = (p.type || 'http').toUpperCase();
                            return `<option value="${escapeHtml(p.id)}" ${p.id === boundPoolId ? 'selected' : ''}>${escapeHtml(displayName)} (${pType})</option>`;
                          }).join('')}
                        </select>
                        <!-- Active Toggle -->
                        <button class="secondary-button" data-toggle-conn="${escapeHtml(conn.id)}" data-active="${isItemActive(conn) ? 1 : 0}" style="font-size:10px; padding:4px 8px;">
                          ${isItemActive(conn) ? '<span style="color:var(--lime)">ACTIVE</span>' : '<span style="color:#ff7a7a">DISABLED</span>'}
                        </button>

                        <!-- Delete Action -->
                        <button class="danger-button" data-delete-conn="${escapeHtml(conn.id)}" style="font-size:10px; padding:4px 8px;">Delete</button>
                      </div>
                    </div>
                  `;
                }).join('')}
              </div>
              ${accountPageCount > 1 ? `
                <div style="display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:8px; padding-top:8px; border-top:1px solid var(--line);">
                  <small style="font:10px var(--mono); color:var(--muted);">Showing ${accountOffset + 1}-${Math.min(accountOffset + accountPageSize, conns.length)} of ${conns.length} accounts</small>
                  <div style="display:flex; align-items:center; gap:4px;">
                    <button type="button" class="secondary-button provider-page-btn" data-provider-page="${accountPage - 1}" ${accountPage === 1 ? 'disabled' : ''} style="font-size:9px; padding:3px 7px;">&larr;</button>
                    <span style="font:10px var(--mono); color:var(--text); padding:0 5px;">Page ${accountPage} / ${accountPageCount}</span>
                    <button type="button" class="secondary-button provider-page-btn" data-provider-page="${accountPage + 1}" ${accountPage === accountPageCount ? 'disabled' : ''} style="font-size:9px; padding:3px 7px;">&rarr;</button>
                  </div>
                </div>
              ` : ''}
            </div>
          `}

          <!-- 2. Models & Live Tester Card -->
          <div class="detail-panel card">
            <div class="detail-panel-head">
              <div>
                <h3>Provider Models</h3>
                <span class="kicker">${modelsList.length} Model(s) Available</span>
              </div>
              <div style="display:flex; gap:6px;">
                ${conns.length > 0 ? `
                  <button class="secondary-button" id="btn-import-models-upstream" style="font-size:10.5px; padding:3px 8px;" title="Fetch all model IDs live from provider /models endpoint">
                    📥 Fetch from /models
                  </button>
                ` : ''}
                <button class="solid-button" id="btn-add-custom-model" style="font-size:10.5px; padding:3px 8px;">+ Add Model</button>
              </div>
            </div>

            <div class="models-list-grid">
              ${modelsList.length === 0 ? `
                <p style="color:var(--muted); font-size:12px; padding:8px 0;">No models registered yet for this provider.</p>
              ` : modelsList.map((cleanModelId) => {
                const fullModelId = `${activePrefix}/${cleanModelId}`;
                const isCustom = customModelIdsSet.has(cleanModelId);
                return `
                  <div class="detail-model-row" id="row-model-${escapeHtml(cleanModelId)}">
                    <div style="display:flex; align-items:center; gap:6px; min-width:0;">
                      <code class="model-id-code" title="${escapeHtml(fullModelId)}">${escapeHtml(fullModelId)}</code>
                      ${isCustom ? `<span class="table-badge" style="font-size:7.5px; padding:1px 4px; background:#c8ff6315; border:1px solid #c8ff6344; color:var(--lime);">CUSTOM</span>` : ''}
                    </div>
                    <div class="model-row-actions">
                      <button class="model-test-btn" data-test-model="${escapeHtml(fullModelId)}">Test Model</button>
                      <button class="model-copy-btn" data-copy-text="${escapeHtml(fullModelId)}" title="Copy Full Model ID">&boxbox;</button>
                      ${isCustom ? `<button class="danger-button" data-delete-custom-model="${escapeHtml(cleanModelId)}" style="font-size:9.5px; padding:2px 6px;" title="Delete Custom Model">&times;</button>` : ''}
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        </div>
      </div>
    `;

    bindProviderDetailActions(provId, conns, meta, activePrefix, accountOffset);
  } catch (err) {
    content.innerHTML = emptySurface(`Error loading provider details: ${err.message}`);
  }
}

function bindProviderDetailActions(provId, conns, meta, activePrefix = '', accountOffset = 0) {
  // Back button
  const backBtn = document.querySelector('#btn-back-to-providers');
  if (backBtn) {
    backBtn.onclick = () => {
      window.location.hash = 'providers';
      setView('providers');
    };
  }
  document.querySelectorAll('.provider-page-btn').forEach((btn) => {
    btn.onclick = () => {
      const nextPage = Number(btn.dataset.providerPage);
      if (nextPage >= 1) {
        providerAccountPages.set(provId, nextPage);
        renderProviderDetail(provId);
      }
    };
  });
  // Add account button
  document.querySelectorAll('[data-add-account]').forEach((btn) => {
    btn.onclick = () => {
      const targetProv = btn.dataset.addAccount || provId;
      openProviderModal(targetProv);
    };
  });

  // Reset health button
  document.querySelectorAll('[data-reset-health]').forEach((btn) => {
    btn.onclick = async () => {
      btn.disabled = true;
      try {
        await fetch(`${apiBase}/admin/health/reset?provider=${encodeURIComponent(btn.dataset.resetHealth)}`, { method: 'POST', headers });
        btn.textContent = 'Health Reset!';
        setTimeout(() => { btn.textContent = 'Reset Health'; btn.disabled = false; }, 1500);
      } catch (err) {
        btn.textContent = err.message;
        btn.disabled = false;
      }
    };
  });

  // Priority Swap Up/Down
  document.querySelectorAll('[data-swap-up]').forEach((btn) => {
    btn.onclick = async () => {
      const idx = Number(btn.dataset.swapUp);
      if (idx <= 0) return;
      const c1 = conns[idx];
      const c2 = conns[idx - 1];
      try {
        await Promise.all([
          fetch(`${apiBase}/api/providers/${c1.id}`, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ priority: idx }) }),
          fetch(`${apiBase}/api/providers/${c2.id}`, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ priority: idx + 1 }) })
        ]);
        await renderProviderDetail(provId);
      } catch (err) {
        console.error('Swap failed:', err);
      }
    };
  });

  document.querySelectorAll('[data-swap-down]').forEach((btn) => {
    btn.onclick = async () => {
      const idx = Number(btn.dataset.swapDown);
      if (idx >= conns.length - 1) return;
      const c1 = conns[idx];
      const c2 = conns[idx + 1];
      try {
        await Promise.all([
          fetch(`${apiBase}/api/providers/${c1.id}`, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ priority: idx + 2 }) }),
          fetch(`${apiBase}/api/providers/${c2.id}`, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ priority: idx + 1 }) })
        ]);
        await renderProviderDetail(provId);
      } catch (err) {
        console.error('Swap failed:', err);
      }
    };
  });

  // Toggle Active/Inactive
  document.querySelectorAll('[data-toggle-conn]').forEach((btn) => {
    btn.onclick = async () => {
      const connId = btn.dataset.toggleConn;
      const currentActive = Number(btn.dataset.active) === 1;
      const nextActive = currentActive ? 0 : 1;
      try {
        await fetch(`${apiBase}/api/providers/${connId}`, {
          method: 'PUT',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ isActive: nextActive })
        });
        await renderProviderDetail(provId);
      } catch (err) {
        console.error('Toggle failed:', err);
      }
    };
  });

  // Proxy Pool Assign
  document.querySelectorAll('[data-conn-proxy]').forEach((select) => {
    select.onchange = async () => {
      const connId = select.dataset.connProxy;
      const poolId = select.value;
      try {
        await fetch(`${apiBase}/api/providers/${connId}`, {
          method: 'PUT',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ proxyPoolId: poolId })
        });
      } catch (err) {
        console.error('Proxy update failed:', err);
      }
    };
  });

  // Delete Connection
  document.querySelectorAll('[data-delete-conn]').forEach((btn) => {
    btn.onclick = async () => {
      const connId = btn.dataset.deleteConn;
      const confirmed = await showConfirmModal({
        title: 'Delete Connection',
        kicker: 'DELETE ACCOUNT',
        message: 'Are you sure you want to delete this provider connection account? This action cannot be undone.',
        confirmText: 'Delete Account',
        danger: true
      });
      if (!confirmed) return;
      try {
        const res = await fetch(`${apiBase}/api/providers/${connId}`, {
          method: 'DELETE',
          headers: getHeaders()
        });
        if (!res.ok) {
          const body = await res.text();
          throw new Error(body || `${res.status} ${res.statusText}`);
        }
        showToast('Connection deleted successfully', 'success');
        await renderProviderDetail(provId);
        // Force the next Overview hydration to reconcile the mesh immediately.
        meshProviderSignature = '';
        if (window.location.hash === '#overview') await loadOverview();
      } catch (err) {
        showToast(`Delete failed: ${err.message}`, 'error');
      }
    };
  });

  // Live Model Testing
  document.querySelectorAll('[data-test-model]').forEach((btn) => {
    btn.onclick = async () => {
      const modelId = btn.dataset.testModel;
      btn.className = 'model-test-btn testing';
      btn.textContent = 'Testing...';
      const start = Date.now();
      try {
        const res = await fetch(`${apiBase}/chat/completions`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({
            model: modelId,
            messages: [{ role: 'user', content: 'hi' }],
            stream: false,
            max_tokens: 5
          })
        });
        const latency = Date.now() - start;
        if (res.ok) {
          btn.className = 'model-test-btn ok';
          btn.textContent = `OK (${latency}ms)`;
        } else {
          const errBody = await res.json().catch(() => ({}));
          btn.className = 'model-test-btn error';
          btn.textContent = `Err: ${res.status}`;
          btn.title = errBody.error || res.statusText;
        }
      } catch (err) {
        btn.className = 'model-test-btn error';
        btn.textContent = 'Network Err';
        btn.title = err.message;
      }
    };
  });

  // Copy Model ID
  document.querySelectorAll('[data-copy-text]').forEach((btn) => {
    btn.onclick = async () => {
      await copyText(btn.dataset.copyText);
      const prev = btn.textContent;
      btn.textContent = 'Copied!';
      setTimeout(() => { btn.textContent = prev; }, 1200);
    };
  });

  // Bulk Proxy Action Handlers
  const applyBulkBtn = document.querySelector('#btn-apply-bulk-proxy');
  const bulkSelect = document.querySelector('#bulk-apply-proxy-select');
  if (applyBulkBtn && bulkSelect) {
    applyBulkBtn.onclick = async () => {
      const poolId = bulkSelect.value;
      applyBulkBtn.disabled = true;
      try {
        await Promise.all(conns.map((c) => fetch(`${apiBase}/api/providers/${c.id}`, {
          method: 'PUT',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ proxyPoolId: poolId === '__none__' ? null : poolId })
        })));
        showToast(`Bulk proxy assigned to all ${conns.length} accounts!`, 'success');
        await renderProviderDetail(provId);
      } catch (e) {
        showToast(`Bulk proxy update failed: ${e.message}`, 'error');
      }
    };
  }

  const distributeBtn = document.querySelector('#btn-distribute-proxies');
  if (distributeBtn) {
    distributeBtn.onclick = async () => {
      const poolPayload = await request('/api/proxy-pools').catch(() => ({ proxyPools: [] }));
      const activePools = (poolPayload.proxyPools || []).filter(isItemActive);
      if (activePools.length === 0) return showToast('No active proxy pools available. Deploy or add a proxy in Pools (07) first.', 'error');
      distributeBtn.disabled = true;
      try {
        await Promise.all(conns.map((c, i) => {
          const poolId = activePools[i % activePools.length].id;
          return fetch(`${apiBase}/api/providers/${c.id}`, {
            method: 'PUT',
            headers: { ...headers, 'Content-Type': 'application/json' },
            body: JSON.stringify({ proxyPoolId: poolId })
          });
        }));
        showToast(`Distributed accounts across ${activePools.length} proxy pools!`, 'success');
        await renderProviderDetail(provId);
      } catch (e) {
        showToast(`Distribute failed: ${e.message}`, 'error');
      }
    };
  }

  const resetAllProxiesBtn = document.querySelector('#btn-reset-all-proxies');
  if (resetAllProxiesBtn) {
    resetAllProxiesBtn.onclick = async () => {
      resetAllProxiesBtn.disabled = true;
      try {
        await Promise.all(conns.map((c) => fetch(`${apiBase}/api/providers/${c.id}`, {
          method: 'PUT',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ proxyPoolId: null })
        })));
        showToast('All accounts reset to Direct connection', 'info');
        await renderProviderDetail(provId);
      } catch (e) {
        showToast(`Reset failed: ${e.message}`, 'error');
      }
    };
  }

  // Per-Provider Routing Strategy Selector Listener (Fallback vs Round-Robin with Sticky Limit)
  const provStratSelect = document.querySelector('#prov-routing-strategy-select');
  const provStickyInput = document.querySelector('#prov-sticky-input');
  const provStickyWrap = document.querySelector('#prov-sticky-wrapper');

  const saveProvRoutingStrategy = async () => {
    if (!provStratSelect) return;
    const stratVal = provStratSelect.value;
    const stickyVal = Number(provStickyInput ? provStickyInput.value : 1) || 1;

    try {
      const curSettings = await request('/api/settings').catch(() => ({}));
      const currentStrategies = curSettings.providerStrategies || {};
      const override = { ...(currentStrategies[provId] || {}) };

      if (stratVal === 'round-robin') {
        override.fallbackStrategy = 'round-robin';
        override.stickyRoundRobinLimit = stickyVal;
      } else {
        delete override.fallbackStrategy;
        delete override.stickyRoundRobinLimit;
      }

      const updated = { ...currentStrategies };
      if (Object.keys(override).length === 0) {
        delete updated[provId];
      } else {
        updated[provId] = override;
      }
      await fetch(`${apiBase}/api/settings`, {
        method: 'PUT',
        headers: { ...getHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...curSettings, providerStrategies: updated })
      });
      showToast(`Provider routing strategy saved: ${stratVal.toUpperCase()}`, 'success');
    } catch (e) {
      showToast(`Failed to save routing strategy: ${e.message}`, 'error');
    }
  };

  if (provStratSelect) {
    provStratSelect.onchange = async () => {
      const isRR = provStratSelect.value === 'round-robin';
      if (provStickyWrap) provStickyWrap.style.display = isRR ? 'inline-flex' : 'none';
      await saveProvRoutingStrategy();
    };
  }
  if (provStickyInput) {
    provStickyInput.onchange = async () => {
      await saveProvRoutingStrategy();
    };
  }

  // Provider-wide proxy policy. Non-direct provider modes override account assignments.
  const providerProxyMode = document.querySelector('#provider-proxy-mode');
  const providerProxyPool = document.querySelector('#provider-proxy-pool');
  const saveProviderProxy = document.querySelector('#btn-save-provider-proxy');
  if (providerProxyMode && providerProxyPool) {
    providerProxyMode.onchange = () => {
      providerProxyPool.style.display = providerProxyMode.value === 'fixed' ? '' : 'none';
    };
  }
  if (saveProviderProxy && providerProxyMode && providerProxyPool) {
    saveProviderProxy.onclick = async () => {
      saveProviderProxy.disabled = true;
      const originalText = saveProviderProxy.textContent;
      saveProviderProxy.textContent = 'Saving...';
      try {
        const curSettings = await request('/api/settings').catch(() => ({}));
        const currentStrategies = curSettings.providerStrategies || {};
        const override = { ...(currentStrategies[provId] || {}) };
        const mode = providerProxyMode.value;
        delete override.rotateStrategy;
        delete override.proxyPoolId;
        if (mode === 'fixed') {
          if (providerProxyPool.value === '__none__') throw new Error('Select an active proxy pool first');
          override.proxyPoolId = providerProxyPool.value;
        } else if (mode === 'round-robin' || mode === 'random') {
          override.rotateStrategy = mode;
        }
        const updated = { ...currentStrategies };
        if (Object.keys(override).length === 0) delete updated[provId];
        else updated[provId] = override;
        const response = await fetch(`${apiBase}/api/settings`, {
          method: 'PUT',
          headers: { ...getHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...curSettings, providerStrategies: updated })
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        showToast(`Provider proxy mode saved: ${mode}`, 'success');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Provider proxy update failed: ${err.message}`, 'error');
      } finally {
        saveProviderProxy.disabled = false;
        saveProviderProxy.textContent = originalText;
      }
    };
  }
  // Save Free Node Proxy Settings (OpenCode Zen / Free Providers)
  const saveFreeProxyBtn = document.querySelector('#btn-save-free-proxy');
  const freeProxySelect = document.querySelector('#free-node-proxy-select');
  const freeRotateSelect = document.querySelector('#free-node-rotate-select');
  const freeExplNote = document.querySelector('#free-proxy-explanation-note');
  const freeStatusEl = document.querySelector('#free-proxy-save-status');

  if (freeRotateSelect && freeProxySelect && freeExplNote) {
    freeRotateSelect.onchange = () => {
      const isRot = freeRotateSelect.value !== 'none';
      freeProxySelect.disabled = isRot;
      freeProxySelect.style.opacity = isRot ? '0.45' : '1';
      freeProxySelect.style.cursor = isRot ? 'not-allowed' : 'default';
      if (isRot) {
        freeExplNote.textContent = `🔄 Dynamic rotation is ACTIVE: Requests cycle across all active proxy pools. The single pool selector above is bypassed.`;
      } else {
        const isDirect = freeProxySelect.value === '__none__';
        freeExplNote.textContent = isDirect 
          ? `🔗 Requests connect directly from your machine's local IP.` 
          : `🔗 Requests route through the single fixed proxy pool selected above.`;
      }
    };
  }
  if (saveFreeProxyBtn && freeProxySelect && freeRotateSelect) {
    saveFreeProxyBtn.onclick = async () => {
      saveFreeProxyBtn.disabled = true;
      const origText = saveFreeProxyBtn.innerHTML;
      saveFreeProxyBtn.innerHTML = '<span class="spinner-icon"></span> Saving...';
      if (freeStatusEl) freeStatusEl.textContent = '';
      try {
        const curSettings = await request('/api/settings').catch(() => ({}));
        const currentStrategies = curSettings.providerStrategies || {};
        const override = { ...(currentStrategies[provId] || {}) };

        const poolId = freeProxySelect.value;
        const rotateStrat = freeRotateSelect.value;

        if (poolId === '__none__') delete override.proxyPoolId;
        else override.proxyPoolId = poolId;

        if (rotateStrat === 'none') delete override.rotateStrategy;
        else override.rotateStrategy = rotateStrat;

        const updated = { ...currentStrategies };
        if (Object.keys(override).length === 0) {
          delete updated[provId];
        } else {
          updated[provId] = override;
        }
        await fetch(`${apiBase}/api/settings`, {
          method: 'PUT',
          headers: { ...getHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...curSettings, providerStrategies: updated })
        });
        showToast(`Proxy settings for ${meta.name} saved!`, 'success');
        if (freeStatusEl) freeStatusEl.textContent = '✓ Saved';
        setTimeout(() => { if (freeStatusEl) freeStatusEl.textContent = ''; }, 2000);
      } catch (err) {
        showToast(`Save failed: ${err.message}`, 'error');
      } finally {
        saveFreeProxyBtn.disabled = false;
        saveFreeProxyBtn.innerHTML = origText;
      }
    };
  }

  // Fetch Models from Upstream /models
  const importModelsBtn = document.querySelector('#btn-import-models-upstream');
  if (importModelsBtn) {
    importModelsBtn.onclick = async () => {
      const activeConn = conns.find(isItemActive) || conns[0];
      const targetId = activeConn ? activeConn.id : provId;
      importModelsBtn.disabled = true;
      const origText = importModelsBtn.innerHTML;
      importModelsBtn.innerHTML = '<span class="spinner-icon"></span> Fetching...';
      try {
        const res = await fetch(`${apiBase}/api/providers/${encodeURIComponent(targetId)}/models`, {
          headers: getHeaders()
        });
        const resText = await res.text();
        let data = {};
        try { data = JSON.parse(resText); } catch {}
        if (!res.ok) throw new Error(data.error || resText || `${res.status} ${res.statusText}`);
        const fetchedModels = data.models || data.data || [];
        if (!fetchedModels.length) {
          showToast('No models returned from upstream /models', 'info');
          return;
        }

        let addedCount = 0;
        for (const m of fetchedModels) {
          let mID = typeof m === 'string' ? m.trim() : (m.id || m.name || '').trim();
          if (!mID) continue;
          if (mID.startsWith(`${activePrefix}/`)) mID = mID.slice(activePrefix.length + 1);
          if (!mID) continue;

          // Add to DB custom models with the full model ID preserved
          try {
            await fetch(`${apiBase}/api/custom-models`, {
              method: 'POST',
              headers: { ...headers, 'Content-Type': 'application/json' },
              body: JSON.stringify({
                provider: provId,
                providerAlias: meta.alias || provId,
                id: mID,
                type: 'llm',
                name: mID
              })
            });
            addedCount++;
          } catch {}
        }

        showToast(`Successfully imported ${addedCount} models from upstream!`, 'success');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Fetch models failed: ${err.message}`, 'error');
      } finally {
        importModelsBtn.disabled = false;
        importModelsBtn.innerHTML = origText;
      }
    };
  }

  // Add Custom Model Action
  const addModelBtn = document.querySelector('#btn-add-custom-model');
  if (addModelBtn) {
    addModelBtn.onclick = async () => {
      const modelName = await showPromptModal({
        title: 'Add Custom Model',
        kicker: `CATALOG / ${meta.name.toUpperCase()}`,
        message: `Enter the upstream Model ID for ${meta.name}. It will be registered in SQLite and available across all clients and combos.`,
        label: 'Model Identifier (e.g. gemini-3.7-flash-high, claude-custom-1)',
        placeholder: 'gemini-3.7-flash-high',
        confirmText: 'Add Model'
      });
      if (!modelName) return;
      const cleanId = modelName.trim();
      addModelBtn.disabled = true;
      try {
        const res = await fetch(`${apiBase}/api/custom-models`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ provider: provId, providerAlias: meta.alias || provId, id: cleanId, type: 'llm', name: cleanId })
        });
        const resText = await res.text();
        let resData = {};
        try { resData = JSON.parse(resText); } catch {}
        if (!res.ok) throw new Error(resData.error || resText || `${res.status} ${res.statusText}`);
        showToast(`Custom model "${cleanId}" added!`, 'success');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Failed to add custom model: ${err.message}`, 'error');
      } finally {
        addModelBtn.disabled = false;
      }
    };
  }
  // Delete Provider Node Action (for OpenAI / Anthropic Compatible Nodes)
  const deleteNodeBtn = document.querySelector('#btn-delete-provider-node');
  if (deleteNodeBtn) {
    deleteNodeBtn.onclick = async () => {
      const confirmed = await showConfirmModal({
        title: 'Delete Provider Node',
        kicker: `DELETE NODE / ${meta.name.toUpperCase()}`,
        message: `Are you sure you want to completely delete "${meta.name}" (${activePrefix}/)? All associated API key accounts and custom models for this node will be removed.`,
        confirmText: 'Delete Node',
        danger: true
      });
      if (!confirmed) return;
      deleteNodeBtn.disabled = true;
      try {
        const res = await fetch(`${apiBase}/api/provider-nodes/${encodeURIComponent(provId)}`, {
          method: 'DELETE',
          headers: getHeaders()
        });
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
        showToast(`Provider node "${meta.name}" deleted`, 'info');
        meshProviderSignature = '';
        window.location.hash = 'providers';
        await renderView('providers');
      } catch (err) {
        showToast(`Failed to delete node: ${err.message}`, 'error');
        deleteNodeBtn.disabled = false;
      }
    };
  }
  // Delete Custom Model Action
  document.querySelectorAll('[data-delete-custom-model]').forEach((btn) => {
    btn.onclick = async () => {
      const modelId = btn.dataset.deleteCustomModel;
      const confirmed = await showConfirmModal({
        title: 'Delete Custom Model',
        kicker: `DELETE / ${meta.name.toUpperCase()}`,
        message: `Are you sure you want to remove custom model "${modelId}" from ${meta.name}?`,
        confirmText: 'Delete Model',
        danger: true
      });
      if (!confirmed) return;
      try {
        const res = await fetch(`${apiBase}/api/custom-models`, {
          method: 'DELETE',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ provider: provId, providerAlias: meta.alias || provId, id: modelId, type: 'llm' })
        });
        const resText = await res.text();
        let resData = {};
        try { resData = JSON.parse(resText); } catch {}
        if (!res.ok) throw new Error(resData.error || resText || `${res.status} ${res.statusText}`);
        showToast(`Model "${modelId}" removed`, 'info');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Failed to delete custom model: ${err.message}`, 'error');
      } finally {
        btn.disabled = false;
      }
    };
  });
  // Edit Provider Routing Prefix
  const editPrefixBtn = document.querySelector('#btn-edit-provider-prefix');
  if (editPrefixBtn) {
    editPrefixBtn.onclick = async () => {
      const newPrefix = await showPromptModal({
        title: 'Edit Routing Prefix',
        kicker: `ROUTING / ${meta.name.toUpperCase()}`,
        message: `Enter the incoming model prefix for ${meta.name}. When clients request "prefix/model" (e.g. "${activePrefix}/gemini-3.7-flash"), Zyrouter will route directly to this provider.`,
        label: 'Routing Prefix (e.g. ag, antigravity, my-google)',
        defaultValue: activePrefix,
        confirmText: 'Save Prefix'
      });
      if (newPrefix === null) return;
      const cleanPrefix = newPrefix.trim().toLowerCase();
      if (!cleanPrefix) {
        showToast('Prefix cannot be empty', 'error');
        return;
      }
      editPrefixBtn.disabled = true;
      try {
        await fetch(`${apiBase}/api/provider-prefixes`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ provider: provId, prefix: cleanPrefix })
        });
        showToast(`Routing prefix updated to "${cleanPrefix}/"!`, 'success');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Failed to save prefix: ${err.message}`, 'error');
      } finally {
        editPrefixBtn.disabled = false;
      }
    };
  }

  // Reset Provider Routing Prefix
  const resetPrefixBtn = document.querySelector('#btn-reset-provider-prefix');
  if (resetPrefixBtn) {
    resetPrefixBtn.onclick = async () => {
      const confirmed = await showConfirmModal({
        title: 'Reset Routing Prefix',
        kicker: `RESET / ${meta.name.toUpperCase()}`,
        message: `Reset routing prefix for ${meta.name} back to default "${meta.alias || provId}"?`,
        confirmText: 'Reset to Default',
        danger: false
      });
      if (!confirmed) return;
      resetPrefixBtn.disabled = true;
      try {
        await fetch(`${apiBase}/api/provider-prefixes/${encodeURIComponent(provId)}`, {
          method: 'DELETE',
          headers
        });
        showToast(`Routing prefix reset to default "${meta.alias || provId}/"`, 'info');
        await renderProviderDetail(provId);
      } catch (err) {
        showToast(`Failed to reset prefix: ${err.message}`, 'error');
      }
    };
  }
}

// ─────────────────────────────────────────────────────────────
// 4. PROVIDER-NATIVE CONNECTION MODALS (100% 9router Parity)
// ─────────────────────────────────────────────────────────────
function providerConnectionModal(presetProviderId = 'openai', customNodeMeta = null) {
  let meta = customNodeMeta || KNOWN_PROVIDER_CATALOG.find((p) => p.id === presetProviderId);
  const isCustomNode = presetProviderId.startsWith('openai-compatible') || presetProviderId.startsWith('anthropic-compatible') || (meta && meta.category === 'custom');
  if (!meta) {
    meta = {
      id: presetProviderId,
      name: presetProviderId,
      authType: 'apikey',
      icon: isCustomNode ? '🔌' : '🔌',
      category: isCustomNode ? 'custom' : 'apikey'
    };
  }
  const authType = meta.authType || 'apikey';

  return `
    <form class="inline-form provider-account-form" id="provider-account-form" data-provider-id="${escapeHtml(meta.id)}" data-auth-type="${escapeHtml(authType)}">
      <div class="form-head">
        <div style="display: flex; align-items: center; gap: 12px;">
          ${renderProviderIcon(meta.id, meta.icon)}
          <div>
            <span class="kicker">SETUP CREDENTIAL / ${escapeHtml(meta.name.toUpperCase())}</span>
            <h2>${isCustomNode ? `Add API Key for ${escapeHtml(meta.name)}` : `Connect ${escapeHtml(meta.name)}`}</h2>
          </div>
        </div>
        ${authType === 'apikey' ? `
          <div class="mode-tabs">
            <button type="button" class="mode-tab active" data-tab="single">Single Key</button>
            <button type="button" class="mode-tab" data-tab="bulk">Bulk Import</button>
          </div>
        ` : authType === 'oauth' ? `
          <div class="mode-tabs">
            <button type="button" class="mode-tab active" data-tab="import-token">Token / Auth JSON</button>
            <button type="button" class="mode-tab" data-tab="oauth-flow">OAuth Device Flow</button>
          </div>
        ` : ''}
      </div>

      <!-- 1. GOOGLE CLOUD / ANTIGRAVITY SPECIFIC MODAL -->
      ${meta.id === 'antigravity' ? `
        <div id="tab-pane-import-token" class="tab-pane active">
          <div class="notice-box" style="border-left: 3px solid var(--lime); background: #0c151c; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
            <strong style="color: var(--lime);">🛡️ Google Cloud Code / Antigravity Credentials</strong>
            <p style="margin: 4px 0 0; font-size: 11px; color: #9bb0c1;">Paste file JSON kredensial (e.g. <code>application_default_credentials.json</code>), token dari <code>gcloud auth print-access-token</code>, atau raw access token (<code>ya29...</code>).</p>
          </div>
          
          <label>
            OAuth Access Token / Auth JSON Content
            <textarea name="oauthToken" id="antigravity-json-input" rows="5" placeholder='{\n  "client_id": "...",\n  "client_secret": "...",\n  "refresh_token": "1//...",\n  "type": "authorized_user"\n}' required></textarea>
          </label>

          <div class="form-grid-2">
            <label>
              Account Name
              <input name="name" id="antigravity-name-input" value="Antigravity Account" required />
            </label>
            <label>
              Account Email (Optional)
              <input type="email" name="email" id="antigravity-email-input" placeholder="developer@gmail.com" />
            </label>
          </div>
        </div>

        <div id="tab-pane-oauth-flow" class="tab-pane hidden">
          <div class="notice-box" style="padding: 16px; background: #080d14; border: 1px solid #1f2d3d; border-radius: 8px; text-align: left;">
            <strong style="font-size: 13px; color: var(--text); display: block; margin-bottom: 6px;">Google Cloud OAuth Code Flow</strong>
            <p style="margin: 0 0 14px; font-size: 11px; color: var(--muted); line-height: 1.4;">Klik tombol di bawah untuk membuka halaman izin Google dengan Client ID Antigravity resmi, lalu salin Authorization Code yang didapat.</p>
            
            <button type="button" class="solid-button" id="btn-start-google-oauth" style="margin-bottom: 16px;">🚀 Buka Halaman Izin Google OAuth</button>

            <label style="display:block; margin-top: 10px;">
              Paste Authorization Code / Callback URL
              <input type="text" id="antigravity-auth-code-input" placeholder="Paste kode (4/0A...) atau callback URL di sini..." />
            </label>
            
            <button type="button" class="secondary-button" id="btn-exchange-antigravity-code" style="margin-top: 10px;">⚡ Exchange Code & Hubungkan Akun</button>
            <p id="antigravity-exchange-status" style="font-size: 11px; margin-top: 8px; color: var(--lime);"></p>
          </div>
        </div>
      ` : ''}

      <!-- 2. GITHUB COPILOT / CODEX SPECIFIC MODAL -->
      ${meta.id === 'codex' ? `
        <div id="tab-pane-import-token" class="tab-pane active">
          <div class="notice-box" style="border-left: 3px solid #63a4ff; background: #0b1420; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
            <strong style="color: #63a4ff;">🐙 GitHub Copilot OAuth Token</strong>
            <p style="margin: 4px 0 0; font-size: 11px; color: #9bb0c1;">Paste your GitHub Copilot token (<code>ghu_...</code>) or auth JSON object containing <code>accessToken</code>.</p>
          </div>

          <label>
            GitHub Copilot Token / Auth JSON
            <textarea name="oauthToken" rows="4" placeholder="ghu_1234567890abcdef..." required></textarea>
          </label>

          <div class="form-grid-2">
            <label>
              Account Name
              <input name="name" value="GitHub Copilot Account" required />
            </label>
            <label>
              GitHub Username / Email (Optional)
              <input name="email" placeholder="octocat@github.com" />
            </label>
          </div>
        </div>

        <div id="tab-pane-oauth-flow" class="tab-pane hidden">
          <div class="notice-box" style="padding: 16px; background: #080d14; border: 1px solid #1f2d3d; border-radius: 8px; text-align: left;">
            <strong style="font-size: 13px; color: var(--text); display: block; margin-bottom: 6px;">GitHub Device Code Flow</strong>
            <p style="margin: 0 0 14px; font-size: 11px; color: var(--muted); line-height: 1.4;">Hubungkan akun GitHub Anda secara instan tanpa perlu menyalin token manual.</p>
            
            <button type="button" class="solid-button" id="btn-start-github-device" style="margin-bottom: 14px;">🚀 Mulai GitHub Device Flow</button>

            <div id="github-device-status-box" class="hidden" style="padding: 12px; background: #0e1622; border: 1px solid #283c54; border-radius: 6px; margin-top: 10px;">
              <p style="font-size: 12px; margin: 0 0 8px;">1. Masukkan kode otorisasi berikut di GitHub:</p>
              <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 12px;">
                <code id="github-user-code" style="font-size: 18px; font-weight: bold; color: var(--lime); background: #06090e; padding: 6px 12px; border-radius: 6px; border: 1px solid #203244;">----</code>
                <button type="button" class="secondary-button" id="btn-copy-github-code">Copy Kode</button>
              </div>
              <a href="https://github.com/login/device" target="_blank" class="solid-button" style="display: inline-block; text-decoration: none; padding: 6px 14px; font-size: 12px;">🌐 Buka github.com/login/device</a>
              <p id="github-poll-status" style="font-size: 11px; margin: 10px 0 0; color: #8ea0b0;">Menunggu konfirmasi di GitHub...</p>
            </div>
          </div>
        </div>
      ` : ''}

      <!-- 3. CLAUDE CODE (OAUTH) MODAL -->
      ${meta.id === 'claude' ? `
        <div id="tab-pane-import-token" class="tab-pane active">
          <div class="notice-box" style="border-left: 3px solid #d97757; background: #180f0c; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
            <strong style="color: #d97757;">📜 Claude Code OAuth Token</strong>
            <p style="margin: 4px 0 0; font-size: 11px; color: #cbb4ab;">Paste session token (<code>sk-ant-sid01-...</code>) atau token JSON dari Anthropic.</p>
          </div>

          <label>
            OAuth Code / Session Token
            <textarea name="oauthToken" rows="4" placeholder="Paste Claude Code authorization code or token..." required></textarea>
          </label>

          <div class="form-grid-2">
            <label>
              Account Name
              <input name="name" value="Claude Code Account" required />
            </label>
            <label>
              Account Email (Optional)
              <input type="email" name="email" placeholder="developer@anthropic.com" />
            </label>
          </div>
        </div>

        <div id="tab-pane-oauth-flow" class="tab-pane hidden">
          <div class="notice-box" style="padding: 16px; background: #080d14; border: 1px solid #1f2d3d; border-radius: 8px; text-align: left;">
            <strong style="font-size: 13px; color: var(--text); display: block; margin-bottom: 6px;">Anthropic Claude Code OAuth</strong>
            <p style="margin: 0 0 14px; font-size: 11px; color: var(--muted); line-height: 1.4;">Buka halaman izin Claude AI, lalu salin Authorization Code yang muncul.</p>
            
            <button type="button" class="solid-button" id="btn-start-claude-oauth" style="margin-bottom: 14px;">🚀 Buka Izin Claude AI</button>

            <label style="display:block; margin-top: 10px;">
              Paste Authorization Code
              <input type="text" id="claude-auth-code-input" placeholder="Paste kode otorisasi dari Claude AI..." />
            </label>
            
            <button type="button" class="secondary-button" id="btn-exchange-claude-code" style="margin-top: 10px;">⚡ Exchange Code & Hubungkan Akun</button>
            <p id="claude-exchange-status" style="font-size: 11px; margin-top: 8px; color: var(--lime);"></p>
          </div>
        </div>
      ` : ''}

      <!-- 4. CURSOR IDE MODAL -->
      ${meta.id === 'cursor' ? `
        <div class="notice-box" style="border-left: 3px solid #a855f7; background: #130a1c; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
          <strong style="color: #c084fc;">🖱️ Cursor IDE Session Import</strong>
          <p style="margin: 4px 0 0; font-size: 11px; color: #c4b5d0;">Auto-detect kredensial langsung dari Cursor IDE atau masukkan Access Token & Machine ID.</p>
        </div>

        <div style="margin-bottom: 16px;">
          <button type="button" class="solid-button" id="btn-auto-detect-cursor" style="margin-bottom: 8px;">🔍 Auto-Detect dari Instalasi Cursor</button>
          <p id="cursor-detect-status" style="font-size: 11px; margin: 0; color: var(--muted);"></p>
        </div>

        <div class="form-grid-2">
          <label>
            Cursor Access Token
            <input type="password" name="apiKey" id="cursor-token-input" placeholder="workos_... or cursor jwt token" required />
          </label>
          <label>
            Machine ID (machineId)
            <input name="machineId" id="cursor-machine-input" placeholder="e.g. 1a2b3c4d5e6f..." required />
          </label>
        </div>

        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="Cursor IDE Account" required />
          </label>
          <label>
            Priority
            <input type="number" name="priority" value="10" min="1" max="100" required />
          </label>
        </div>
      ` : ''}

      <!-- 5. IFLOW COOKIE MODAL -->
      ${meta.id === 'iflow' ? `
        <div class="notice-box" style="border-left: 3px solid #eab308; background: #171306; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
          <strong style="color: #fde047;">🍪 iFlow Browser Session Cookie</strong>
          <p style="margin: 4px 0 0; font-size: 11px; color: #d6cfb3;">Open DevTools (F12) &rarr; Application &rarr; Cookies &rarr; Copy the session cookie (<code>sso=...</code>).</p>
        </div>

        <label>
          Session Cookie String
          <textarea name="sessionCookie" rows="3" placeholder="sso=eyJhbGciOi..." required></textarea>
        </label>

        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="iFlow Account" required />
          </label>
          <label>
            Account Email / ID (Optional)
            <input name="email" placeholder="user@domain.com" />
          </label>
        </div>
      ` : ''}

      <!-- 6. LOCAL & NO-AUTH PROVIDERS (Ollama, LM Studio, vLLM) -->
      ${authType === 'noauth' ? `
        <div class="notice-box" style="border-left: 3px solid var(--lime); background: #0c151c; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
          <strong style="color: var(--lime);">🦙 Local Inference Engine</strong>
          <p style="margin: 4px 0 0; font-size: 11px; color: #9bb0c1;">Connects directly to your locally running model server without requiring API keys.</p>
        </div>

        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="${escapeHtml(meta.name)} Node" required />
          </label>
          <label>
            Host Base URL
            <input name="baseUrl" value="${escapeHtml(meta.defaultUrl || 'http://localhost:11434')}" placeholder="http://localhost:11434" required />
          </label>
        </div>

        <div class="form-grid-2">
          <label>
            Custom Models (Optional, comma-separated)
            <input name="customModels" placeholder="e.g. llama3.3:70b, deepseek-r1:14b" />
          </label>
          <label>
            Priority (1 = Highest)
            <input type="number" name="priority" value="10" min="1" max="100" required />
          </label>
        </div>
      ` : ''}

      <!-- 7. FREE 1-CLICK ENABLE FORM (OpenCode Zen, DuckDuckGo) -->
      ${authType === 'free' ? `
        <div class="notice-box" style="border-left: 3px solid var(--lime); background: #0c151c; padding: 14px; border-radius: 6px; margin-bottom: 14px;">
          <strong style="color: var(--lime); font-size: 13px;">🎁 Free Community Endpoint</strong>
          <p style="margin: 6px 0 0; font-size: 12px; color: #a5b9cb; line-height: 1.4;">This provider requires no personal subscription or private API key. Click Save to activate public gateway routing immediately.</p>
        </div>
        <input type="hidden" name="name" value="${escapeHtml(meta.name)} Free Node" />
        <input type="hidden" name="apiKey" value="public" />
        <input type="hidden" name="priority" value="10" />
      ` : ''}

      <!-- 8. AZURE OPENAI FORM -->
      ${authType === 'azure' ? `
        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="Azure OpenAI Account" required />
          </label>
          <label>
            Azure Resource Endpoint
            <input name="azureEndpoint" placeholder="https://your-resource.openai.azure.com/" required />
          </label>
        </div>
        <div class="form-grid-2">
          <label>
            Azure API Key
            <input type="password" name="apiKey" placeholder="Azure Resource API Key" required />
          </label>
          <label>
            API Version
            <input name="apiVersion" value="2024-10-01-preview" required />
          </label>
        </div>
        <div class="form-grid-2">
          <label>
            Deployment Name (Model route)
            <input name="deployment" placeholder="e.g. gpt-4o" required />
          </label>
          <label>
            Priority
            <input type="number" name="priority" value="10" min="1" max="100" required />
          </label>
        </div>
      ` : ''}

      <!-- 9. CLOUDFLARE WORKERS AI FORM -->
      ${authType === 'cloudflare' ? `
        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="Cloudflare AI Account" required />
          </label>
          <label>
            Cloudflare Account ID
            <input name="accountId" placeholder="e.g. 1a2b3c4d5e..." required />
          </label>
        </div>
        <div class="form-grid-2">
          <label>
            API Token
            <input type="password" name="apiKey" placeholder="Cloudflare API Token" required />
          </label>
          <label>
            Priority
            <input type="number" name="priority" value="10" min="1" max="100" required />
          </label>
        </div>
      ` : ''}

      <!-- 10. CUSTOM COMPATIBLE FORM -->
      ${(authType === 'custom-openai' || authType === 'custom-anthropic') ? `
        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="Custom ${escapeHtml(meta.name)} Node" required />
          </label>
          <label>
            Base URL
            <input name="baseUrl" placeholder="https://api.example.com/v1" required />
          </label>
        </div>
        <div class="form-grid-2">
          <label>
            API Key / Bearer Token (Optional)
            <input type="password" name="apiKey" placeholder="Leave empty if open endpoint" />
          </label>
          <label>
            Default Model
            <input name="defaultModel" placeholder="e.g. custom-model-1" />
          </label>
        </div>
        <div class="form-grid-2">
          <label>
            Custom Models (comma-separated)
            <input name="customModels" placeholder="e.g. model-a, model-b" />
          </label>
          <label>
            Priority
            <input type="number" name="priority" value="10" min="1" max="100" required />
          </label>
        </div>
      ` : ''}

      <!-- 11. OTHER OAUTH PROVIDERS (Kiro, Qoder, GitLab, Windsurf, Trae, Cline, Devin, Kimi, Zed) -->
      ${(authType === 'oauth' && meta.id !== 'antigravity' && meta.id !== 'codex' && meta.id !== 'claude' && meta.id !== 'cursor') ? `
        <div class="notice-box" style="border-left: 3px solid var(--lime); background: #0c151c; padding: 12px 14px; border-radius: 6px; margin-bottom: 14px;">
          <strong style="color: var(--lime);">🛡️ ${escapeHtml(meta.name)} Token / Session Import</strong>
          <p style="margin: 4px 0 0; font-size: 11px; color: #9bb0c1;">Paste the OAuth Access Token, Session Key, or JSON configuration.</p>
        </div>

        <label>
          OAuth Token / Auth JSON Content
          <textarea name="oauthToken" rows="4" placeholder="Paste access_token or full auth JSON here..." required></textarea>
        </label>

        <div class="form-grid-2">
          <label>
            Connection Name
            <input name="name" value="${escapeHtml(meta.name)} Account" required />
          </label>
          <label>
            Account Email (Optional)
            <input type="email" name="email" placeholder="developer@domain.com" />
          </label>
        </div>
      ` : ''}

      <!-- 12. STANDARD & CUSTOM COMPATIBLE API KEY FORM (Single & Bulk Tabs) -->
      ${(authType === 'apikey' && meta.id !== 'azure' && meta.id !== 'cloudflare-ai' && meta.id !== 'iflow' && meta.id !== 'grok-web' && meta.id !== 'custom-embedding') ? `
        <div id="tab-single-key-content" class="tab-pane active">
          <div class="form-grid-2">
            <label>
              Key Name / Label
              <input name="name" value="Primary ${escapeHtml(meta.name)} Key" required />
            </label>
            <label>
              API Key
              <input type="password" name="apiKey" placeholder="${escapeHtml(meta.keyPlaceholder || (isCustomNode ? 'sk-...' : 'sk-...'))}" required />
            </label>
          </div>
          ${!isCustomNode ? `
            <div class="form-grid-2">
              <label>
                Base URL Override (Optional)
                <input name="baseUrl" placeholder="Leave empty for official endpoint" />
              </label>
              <label>
                Priority (1 = Highest)
                <input type="number" name="priority" value="10" min="1" max="100" required />
              </label>
            </div>
            <div class="form-grid-2">
              <label>
                Account Email (Optional)
                <input type="email" name="email" placeholder="account@domain.com" />
              </label>
            </div>
          ` : `
            <div class="form-grid-2">
              <label>
                Priority (1 = Highest)
                <input type="number" name="priority" value="10" min="1" max="100" required />
              </label>
            </div>
          `}
        </div>

        <div id="tab-bulk-key-content" class="tab-pane hidden">
          <label>
            Bulk Key Paste (Format: <code>name|key</code> or just <code>key</code> per line)
            <textarea name="bulkKeys" rows="6" placeholder="Key1|sk-proj-123456\nKey2|sk-proj-789012\nsk-proj-only-auto-named"></textarea>
          </label>
        </div>
      ` : ''}
      <!-- PRIORITY & ACTIONS -->
      <div class="form-actions" style="margin-top: 18px;">
        <button class="solid-button" type="submit">Save Provider Node</button>
        <button class="cancel-button" type="button" id="btn-cancel-provider-modal">Cancel</button>
      </div>
      <p class="form-error" role="alert"></p>
    </form>
  `;
}

async function openProviderModal(provId = 'openai') {
  const existing = document.querySelector('#provider-account-form');
  if (existing) existing.remove();

  let customNodeMeta = null;
  if (provId.startsWith('openai-compatible') || provId.startsWith('anthropic-compatible')) {
    try {
      const nodesData = await request('/api/provider-nodes');
      const node = (nodesData.nodes || []).find((n) => n.id === provId);
      if (node) {
        customNodeMeta = {
          id: node.id,
          name: node.name || (node.type === 'anthropic-compatible' ? 'Anthropic Compatible' : 'OpenAI Compatible'),
          desc: node.baseUrl || 'Custom Endpoint',
          icon: node.type === 'anthropic-compatible' ? '🎭' : '🔌',
          category: 'custom',
          authType: 'apikey'
        };
      }
    } catch {}
  }

  content.insertAdjacentHTML('afterbegin', providerConnectionModal(provId, customNodeMeta));
  const form = document.querySelector('#provider-account-form');
  if (!form) return;

  // Names and priorities are generated by the backend from account data/order.
  // Keep these fields optional so bulk imports do not receive artificial defaults.
  form.querySelectorAll('input[name="name"], input[name="priority"]').forEach((input) => {
    input.value = '';
    input.required = false;
    input.placeholder = input.name === 'name' ? 'Auto-generated from email/key' : 'Auto (next priority)';
  });

  // Tab switching (Single vs Bulk or Import vs Flow)
  form.querySelectorAll('.mode-tab').forEach((tab) => {
    tab.onclick = () => {
      form.querySelectorAll('.mode-tab').forEach((t) => t.classList.remove('active'));
      tab.classList.add('active');
      const tabName = tab.dataset.tab;
      
      const isSingle = tabName === 'single' || tabName === 'import-token';
      const pane1 = form.querySelector('#tab-single-key-content') || form.querySelector('#tab-pane-import-token');
      const pane2 = form.querySelector('#tab-bulk-key-content') || form.querySelector('#tab-pane-oauth-flow');
      if (pane1) pane1.classList.toggle('hidden', !isSingle);
      if (pane2) pane2.classList.toggle('hidden', isSingle);
    };
  });

  // Auto-parse Antigravity JSON input
  const agJsonInput = form.querySelector('#antigravity-json-input');
  if (agJsonInput) {
    agJsonInput.oninput = () => {
      const val = agJsonInput.value.trim();
      if (val.startsWith('{') && val.endsWith('}')) {
        try {
          const parsed = JSON.parse(val);
          if (parsed.email && form.querySelector('#antigravity-email-input')) {
            form.querySelector('#antigravity-email-input').value = parsed.email;
          }
          if (parsed.client_email && form.querySelector('#antigravity-email-input')) {
            form.querySelector('#antigravity-email-input').value = parsed.client_email;
          }
          if (parsed.project_id && form.querySelector('#antigravity-name-input')) {
            form.querySelector('#antigravity-name-input').value = `Antigravity (${parsed.project_id})`;
          }
        } catch {}
      }
    };
  }

  // Antigravity Google OAuth interactive flow
  const btnStartGoogle = form.querySelector('#btn-start-google-oauth');
  if (btnStartGoogle) {
    btnStartGoogle.onclick = async () => {
      btnStartGoogle.disabled = true;
      btnStartGoogle.textContent = 'Menyiapkan URL Izin Google...';
      try {
        const authData = await request('/api/oauth/antigravity/authorize');
        if (authData.authUrl) {
          window.open(authData.authUrl, '_blank');
          btnStartGoogle.textContent = '🚀 Buka Ulang Izin Google';
          btnStartGoogle.disabled = false;
        }
      } catch (err) {
        btnStartGoogle.textContent = `Gagal: ${err.message}`;
        btnStartGoogle.disabled = false;
      }
    };
  }

  const btnExchangeAg = form.querySelector('#btn-exchange-antigravity-code');
  if (btnExchangeAg) {
    btnExchangeAg.onclick = async () => {
      const input = form.querySelector('#antigravity-auth-code-input');
      const statusEl = form.querySelector('#antigravity-exchange-status');
      let rawCode = (input?.value || '').trim();
      if (!rawCode) {
        statusEl.style.color = '#ff8787';
        statusEl.textContent = 'Harap masukkan Authorization Code atau Callback URL';
        return;
      }

      // Extract code if user pasted full callback URL
      if (rawCode.includes('code=')) {
        try {
          const urlObj = new URL(rawCode);
          rawCode = urlObj.searchParams.get('code') || rawCode;
        } catch {
          const match = rawCode.match(/code=([^&]+)/);
          if (match) rawCode = decodeURIComponent(match[1]);
        }
      }

      btnExchangeAg.disabled = true;
      statusEl.style.color = 'var(--lime)';
      statusEl.textContent = 'Menghubungkan ke Google Token Endpoint...';

      try {
        const res = await fetch(`${apiBase}/api/oauth/antigravity/exchange`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ code: rawCode })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Exchange token gagal');

        statusEl.textContent = `Berhasil terhubung sebagai ${data.email || data.name}!`;
        setTimeout(async () => {
          form.remove();
          if (window.location.hash.startsWith('#provider/')) {
            await renderProviderDetail('antigravity');
          } else {
            await renderView('providers');
          }
        }, 1000);
      } catch (err) {
        btnExchangeAg.disabled = false;
        statusEl.style.color = '#ff8787';
        statusEl.textContent = `Gagal: ${err.message}`;
      }
    };
  }

  // GitHub Copilot Device Flow interactive logic
  const btnStartGithub = form.querySelector('#btn-start-github-device');
  if (btnStartGithub) {
    let pollTimer = null;
    btnStartGithub.onclick = async () => {
      btnStartGithub.disabled = true;
      btnStartGithub.textContent = 'Meminta Device Code...';
      try {
        const res = await fetch(`${apiBase}/api/oauth/github/device-code`, { method: 'POST', headers });
        const data = await res.json();
        if (!res.ok || !data.device_code) throw new Error(data.error || 'Gagal meminta device code');

        const box = form.querySelector('#github-device-status-box');
        const codeEl = form.querySelector('#github-user-code');
        const pollStatus = form.querySelector('#github-poll-status');
        if (box) box.classList.remove('hidden');
        if (codeEl) codeEl.textContent = data.user_code || '----';

        btnStartGithub.textContent = '🔄 Mulai Ulang Device Code';
        btnStartGithub.disabled = false;

        const copyBtn = form.querySelector('#btn-copy-github-code');
        if (copyBtn) {
          copyBtn.onclick = async () => {
            await copyText(data.user_code);
            copyBtn.textContent = 'Tersalin!';
            setTimeout(() => { copyBtn.textContent = 'Copy Kode'; }, 1500);
          };
        }

        // Start background polling
        const intervalMs = Math.max(5, Number(data.interval) || 5) * 1000;
        if (pollTimer) clearInterval(pollTimer);

        pollTimer = setInterval(async () => {
          try {
            const pRes = await fetch(`${apiBase}/api/oauth/github/poll`, {
              method: 'POST',
              headers: { ...headers, 'Content-Type': 'application/json' },
              body: JSON.stringify({ deviceCode: data.device_code })
            });
            const pData = await pRes.json();
            if (pData.access_token) {
              clearInterval(pollTimer);
              if (pollStatus) {
                pollStatus.style.color = 'var(--lime)';
                pollStatus.textContent = '🎉 Berhasil terhubung ke GitHub Copilot!';
              }
              setTimeout(async () => {
                form.remove();
                if (window.location.hash.startsWith('#provider/')) {
                  await renderProviderDetail('codex');
                } else {
                  await renderView('providers');
                }
              }, 1200);
            } else if (pData.error === 'authorization_pending') {
              if (pollStatus) pollStatus.textContent = 'Menunggu Anda mengizinkan kode di browser...';
            } else if (pData.error === 'slow_down') {
              if (pollStatus) pollStatus.textContent = 'Memperlambat polling...';
            } else if (pData.error) {
              clearInterval(pollTimer);
              if (pollStatus) {
                pollStatus.style.color = '#ff8787';
                pollStatus.textContent = `Polling error: ${pData.error_description || pData.error}`;
              }
            }
          } catch (err) {
            console.debug('GitHub poll error:', err);
          }
        }, intervalMs);
      } catch (err) {
        btnStartGithub.disabled = false;
        btnStartGithub.textContent = `Gagal: ${err.message}`;
      }
    };
  }

  // Claude Code OAuth Flow logic
  const btnStartClaude = form.querySelector('#btn-start-claude-oauth');
  if (btnStartClaude) {
    let claudeVerifier = '';
    btnStartClaude.onclick = async () => {
      btnStartClaude.disabled = true;
      btnStartClaude.textContent = 'Menyiapkan URL Claude...';
      try {
        const authData = await request('/api/oauth/claude/authorize');
        if (authData.authUrl) {
          claudeVerifier = authData.codeVerifier;
          window.open(authData.authUrl, '_blank');
          btnStartClaude.textContent = '🚀 Buka Ulang Izin Claude AI';
          btnStartClaude.disabled = false;
        }
      } catch (err) {
        btnStartClaude.textContent = `Gagal: ${err.message}`;
        btnStartClaude.disabled = false;
      }
    };

    const btnExchangeClaude = form.querySelector('#btn-exchange-claude-code');
    if (btnExchangeClaude) {
      btnExchangeClaude.onclick = async () => {
        const input = form.querySelector('#claude-auth-code-input');
        const statusEl = form.querySelector('#claude-exchange-status');
        const rawCode = (input?.value || '').trim();
        if (!rawCode) {
          statusEl.style.color = '#ff8787';
          statusEl.textContent = 'Harap masukkan Authorization Code dari Claude';
          return;
        }

        btnExchangeClaude.disabled = true;
        statusEl.style.color = 'var(--lime)';
        statusEl.textContent = 'Menghubungkan ke Anthropic...';

        try {
          const res = await fetch(`${apiBase}/api/oauth/claude/exchange`, {
            method: 'POST',
            headers: { ...headers, 'Content-Type': 'application/json' },
            body: JSON.stringify({ code: rawCode, codeVerifier: claudeVerifier })
          });
          const data = await res.json();
          if (!res.ok) throw new Error(data.error || 'Exchange token gagal');

          statusEl.textContent = 'Berhasil terhubung ke Claude Code!';
          setTimeout(async () => {
            form.remove();
            if (window.location.hash.startsWith('#provider/')) {
              await renderProviderDetail('claude');
            } else {
              await renderView('providers');
            }
          }, 1000);
        } catch (err) {
          btnExchangeClaude.disabled = false;
          statusEl.style.color = '#ff8787';
          statusEl.textContent = `Gagal: ${err.message}`;
        }
      };
    }
  }

  // Cursor Auto-Detect Logic
  const btnAutoCursor = form.querySelector('#btn-auto-detect-cursor');
  if (btnAutoCursor) {
    btnAutoCursor.onclick = async () => {
      btnAutoCursor.disabled = true;
      btnAutoCursor.textContent = 'Mencari state.vscdb...';
      const statusEl = form.querySelector('#cursor-detect-status');
      try {
        const res = await fetch(`${apiBase}/api/oauth/cursor/auto-import`);
        const data = await res.json();
        if (data.found && data.accessToken) {
          form.querySelector('#cursor-token-input').value = data.accessToken;
          if (data.machineId && form.querySelector('#cursor-machine-input')) {
            form.querySelector('#cursor-machine-input').value = data.machineId;
          }
          statusEl.style.color = 'var(--lime)';
          statusEl.textContent = '✓ Berhasil terdeteksi & disimpan langsung dari instalasi Cursor!';
          btnAutoCursor.textContent = '✓ Terdeteksi!';
          setTimeout(async () => {
            form.remove();
            if (window.location.hash.startsWith('#provider/')) {
              await renderProviderDetail('cursor');
            } else {
              await renderView('providers');
            }
          }, 1200);
        } else {
          statusEl.style.color = '#ff8787';
          statusEl.textContent = data.error || 'Tidak menemukan token di file instalasi Cursor.';
          btnAutoCursor.disabled = false;
          btnAutoCursor.textContent = '🔍 Coba Lagi';
        }
      } catch (err) {
        btnAutoCursor.disabled = false;
        statusEl.style.color = '#ff8787';
        statusEl.textContent = `Error: ${err.message}`;
      }
    };
  }

  const cancelBtn = form.querySelector('#btn-cancel-provider-modal');
  const submitBtn = form.querySelector('button[type="submit"]');
  let isSavingProvider = false;

  cancelBtn.onclick = () => {
    if (isSavingProvider) return;
    form.remove();
  };

  form.onsubmit = async (event) => {
    event.preventDefault();
    if (isSavingProvider) return; // Anti-spam lock

    isSavingProvider = true;
    submitBtn.disabled = true;
    cancelBtn.disabled = true;
    const originalBtnHtml = submitBtn.innerHTML;
    submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving Provider...';
    form.querySelector('.form-error').textContent = '';

    const values = Object.fromEntries(new FormData(form).entries());
    const selectedProv = provId;
    const meta = KNOWN_PROVIDER_CATALOG.find((p) => p.id === selectedProv) || { authType: 'apikey' };
    const authType = meta.authType || 'apikey';

    // Bulk Import Mode
    const activeTab = form.querySelector('.mode-tab.active');
    const isBulk = activeTab && activeTab.dataset.tab === 'bulk' && values.bulkKeys?.trim();

    if (isBulk) {
      const lines = values.bulkKeys.split('\n').map((l) => l.trim()).filter(Boolean);
      if (!lines.length) {
        form.querySelector('.form-error').textContent = 'Please paste at least one key for bulk import';
        isSavingProvider = false;
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalBtnHtml;
        cancelBtn.disabled = false;
        return;
      }

      try {
        for (let i = 0; i < lines.length; i++) {
          const line = lines[i];
          let name = `${values.name || selectedProv} #${i + 1}`;
          let key = line;
          if (line.includes('|')) {
            const parts = line.split('|');
            name = parts[0].trim();
            key = parts[1].trim();
          }

          const dataObj = { apiKey: key };
          await fetch(`${apiBase}/api/providers`, {
            method: 'POST',
            headers: { ...headers, 'Content-Type': 'application/json' },
            body: JSON.stringify({
              provider: selectedProv,
              name: name,
              authType: 'apikey',
              name: values.name?.trim() || undefined,
              priority: values.priority ? Number(values.priority) : undefined,
              data: JSON.stringify(dataObj)
            })
          });
        }
        form.remove();
        if (window.location.hash.startsWith('#provider/')) {
          await renderProviderDetail(selectedProv);
        } else {
          await renderView('providers');
        }
      } catch (err) {
        form.querySelector('.form-error').textContent = `Bulk import error: ${err.message}`;
      } finally {
        isSavingProvider = false;
        if (document.body.contains(form)) {
          submitBtn.disabled = false;
          submitBtn.innerHTML = originalBtnHtml;
          cancelBtn.disabled = false;
        }
      }
      return;
    }
    // Modality-specific creation
    let authTypeToSave = 'apikey';
    const dataObj = {};

    if (authType === 'noauth') {
      authTypeToSave = 'noauth';
      dataObj.baseUrl = values.baseUrl || meta.defaultUrl || 'http://localhost:11434';
      if (values.apiKey) dataObj.apiKey = values.apiKey;
      if (values.customModels) {
        dataObj.customModels = values.customModels.split(',').map((s) => s.trim()).filter(Boolean);
      }
    } else if (authType === 'free') {
      authTypeToSave = 'apikey';
      dataObj.apiKey = 'public';
      if (meta.defaultUrl) dataObj.baseUrl = meta.defaultUrl;
    } else if (authType === 'azure') {
      authTypeToSave = 'apikey';
      dataObj.apiKey = values.apiKey;
      dataObj.azureEndpoint = values.azureEndpoint;
      dataObj.apiVersion = values.apiVersion || '2024-10-01-preview';
      dataObj.deployment = values.deployment;
    } else if (authType === 'cloudflare') {
      authTypeToSave = 'apikey';
      dataObj.apiKey = values.apiKey;
      dataObj.accountId = values.accountId;
    } else if (authType === 'oauth') {
      authTypeToSave = 'oauth';
      let rawToken = values.oauthToken || '';
      if (rawToken.startsWith('{') && rawToken.endsWith('}')) {
        try {
          const parsed = JSON.parse(rawToken);
          Object.assign(dataObj, parsed);
          dataObj.apiKey = parsed.access_token || parsed.accessToken || parsed.token || rawToken;
          dataObj.accessToken = parsed.access_token || parsed.accessToken || parsed.token || rawToken;
          if (parsed.refresh_token || parsed.refreshToken) {
            dataObj.refreshToken = parsed.refresh_token || parsed.refreshToken;
          }
        } catch {
          dataObj.apiKey = rawToken;
          dataObj.accessToken = rawToken;
        }
      } else {
        dataObj.apiKey = rawToken;
        dataObj.accessToken = rawToken;
      }
      if (values.machineId) dataObj.machineId = values.machineId;
    } else if (authType === 'cookie') {
      authTypeToSave = 'cookie';
      dataObj.sessionCookie = values.sessionCookie;
      dataObj.apiKey = values.sessionCookie;
    } else if (authType === 'custom-openai' || authType === 'custom-anthropic') {
      authTypeToSave = 'apikey';
      dataObj.baseUrl = values.baseUrl;
      if (values.apiKey) dataObj.apiKey = values.apiKey;
      if (values.defaultModel) dataObj.defaultModel = values.defaultModel;
      if (values.customModels) {
        dataObj.customModels = values.customModels.split(',').map((s) => s.trim()).filter(Boolean);
      }
    } else {
      authTypeToSave = 'apikey';
      dataObj.apiKey = values.apiKey;
      if (values.baseUrl) dataObj.baseUrl = values.baseUrl;
    }

    try {
      const response = await fetch(`${apiBase}/api/providers`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: selectedProv,
          name: values.name?.trim() || undefined,
          authType: authTypeToSave,
          email: values.email || undefined,
          priority: values.priority ? Number(values.priority) : undefined,
          data: JSON.stringify(dataObj)
        })
      });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || `${response.status} ${response.statusText}`);
      form.remove();
      if (window.location.hash.startsWith('#provider/')) {
        await renderProviderDetail(selectedProv);
      } else {
        await renderView('providers');
      }
    } catch (err) {
      form.querySelector('.form-error').textContent = err.message;
    } finally {
      isSavingProvider = false;
      if (document.body.contains(form)) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalBtnHtml;
        cancelBtn.disabled = false;
      }
    }
  };
}

function bindProviderGroupActions() {
  const mainAddBtn = document.querySelector('#btn-open-add-provider');
  if (mainAddBtn) mainAddBtn.onclick = () => openProviderModal('openai');

  const addOpenAINodeBtn = document.querySelector('#btn-add-openai-node');
  if (addOpenAINodeBtn) {
    addOpenAINodeBtn.onclick = () => openCompatibleNodeModal('openai');
  }

  const addAnthropicNodeBtn = document.querySelector('#btn-add-anthropic-node');
  if (addAnthropicNodeBtn) {
    addAnthropicNodeBtn.onclick = () => openCompatibleNodeModal('anthropic');
  }

  document.querySelectorAll('[data-open-provider]').forEach((card) => {
    card.onclick = () => {
      const provId = card.dataset.openProvider;
      window.location.hash = `provider/${provId}`;
      renderProviderDetail(provId);
    };
  });
}
function openCompatibleNodeModal(variant = 'openai', existingNode = null) {
  const isAnthropic = variant === 'anthropic' || (existingNode && existingNode.type === 'anthropic-compatible');
  const isEdit = !!existingNode;
  const existing = document.querySelector('#compatible-node-modal');
  if (existing) existing.remove();

  const title = isEdit
    ? `Edit ${isAnthropic ? 'Anthropic' : 'OpenAI'} Compatible Node`
    : `Add ${isAnthropic ? 'Anthropic' : 'OpenAI'} Compatible Node`;
  const defaultBaseUrl = isAnthropic ? 'https://api.anthropic.com/v1' : 'https://api.openai.com/v1';

  const html = `
    <div id="compatible-node-modal" class="modal-backdrop">
      <div class="cyber-modal-card" style="max-width:520px;">
        <div class="cyber-modal-head">
          <div>
            <span class="kicker" style="font-size:8px;">CUSTOM RUNTIME NODE</span>
            <h3>${escapeHtml(title)}</h3>
          </div>
          <button type="button" class="cancel-button" id="btn-close-node-modal" style="padding:2px 6px;">&times;</button>
        </div>
        <form id="compatible-node-form" style="display:grid; gap:12px; padding:16px;">
          <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Node Name
            <input name="name" value="${escapeHtml(existingNode?.name || '')}" placeholder="${isAnthropic ? 'Anthropic Compatible (Prod)' : 'OpenAI Compatible (Prod)'}" required style="background:#05070a; border:1px solid var(--line); padding:8px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
            <span style="font-size:9.5px; color:#5a6e82; text-transform:none;">A friendly display label for this custom endpoint node.</span>
          </label>

          <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Routing Prefix
            <input name="prefix" value="${escapeHtml(existingNode?.prefix || '')}" placeholder="${isAnthropic ? 'ac-prod' : 'oc-prod'}" required style="background:#05070a; border:1px solid var(--line); padding:8px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
            <span style="font-size:9.5px; color:#5a6e82; text-transform:none;">Used as prefix for model IDs (e.g. <code>prefix/model-name</code>).</span>
          </label>

          ${!isAnthropic ? `
            <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
              API Type
              <select name="apiType" style="background:#05070a; border:1px solid var(--line); padding:8px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;">
                <option value="chat" ${existingNode?.apiType === 'chat' ? 'selected' : ''}>Chat Completions (/v1/chat/completions)</option>
                <option value="responses" ${existingNode?.apiType === 'responses' ? 'selected' : ''}>Responses API (/v1/responses)</option>
              </select>
            </label>
          ` : ''}

          <label style="display:grid; gap:4px; font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Base URL
            <input name="baseUrl" value="${escapeHtml(existingNode?.baseUrl || defaultBaseUrl)}" placeholder="${escapeHtml(defaultBaseUrl)}" required style="background:#05070a; border:1px solid var(--line); padding:8px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
            <span style="font-size:9.5px; color:#5a6e82; text-transform:none;">Use the base URL (ending in /v1).</span>
          </label>

          <div style="background:#080d14; border:1px solid var(--line); border-radius:6px; padding:10px 12px; margin-top:2px;">
            <span class="kicker" style="font-size:8px;">CONNECTIVITY CHECK (OPTIONAL)</span>
            <div style="display:grid; grid-template-columns:1fr auto; gap:8px; margin-top:6px;">
              <input type="password" id="node-check-key" placeholder="API Key for test (optional)" style="background:#05070a; border:1px solid var(--line); padding:6px 10px; font:10px var(--mono); color:var(--text); border-radius:5px;" />
              <button type="button" class="secondary-button" id="btn-validate-node" style="font-size:10px; padding:6px 12px;">Test Connection</button>
            </div>
            <p id="node-validate-status" style="font-size:10.5px; font-family:var(--mono); margin:6px 0 0;"></p>
          </div>

          <p class="form-error" id="node-modal-error" style="font-size:11px; color:var(--danger); margin:0;"></p>

          <div class="cyber-modal-actions" style="margin-top:6px;">
            <button type="button" class="cancel-button" id="btn-cancel-node-modal">Cancel</button>
            <button type="submit" class="solid-button" id="btn-submit-node-modal">${isEdit ? 'Save Changes' : 'Create Node'}</button>
          </div>
        </form>
      </div>
    </div>
  `;

  document.body.insertAdjacentHTML('beforeend', html);
  const modalEl = document.querySelector('#compatible-node-modal');
  const form = modalEl.querySelector('#compatible-node-form');
  const closeBtn = modalEl.querySelector('#btn-close-node-modal');
  const cancelBtn = modalEl.querySelector('#btn-cancel-node-modal');
  const submitBtn = modalEl.querySelector('#btn-submit-node-modal');
  const valBtn = modalEl.querySelector('#btn-validate-node');
  const valStatus = modalEl.querySelector('#node-validate-status');
  const errEl = modalEl.querySelector('#node-modal-error');

  const cleanup = () => modalEl.remove();
  if (closeBtn) closeBtn.onclick = cleanup;
  if (cancelBtn) cancelBtn.onclick = cleanup;

  if (valBtn) {
    valBtn.onclick = async () => {
      const baseUrl = form.querySelector('[name="baseUrl"]').value.trim();
      const apiKey = form.querySelector('#node-check-key').value.trim();
      if (!baseUrl) {
        valStatus.style.color = 'var(--danger)';
        valStatus.textContent = 'Please enter Base URL first';
        return;
      }
      valBtn.disabled = true;
      valStatus.style.color = 'var(--lime)';
      valStatus.innerHTML = '<span class="spinner-icon"></span> Testing connection...';
      try {
        const res = await fetch(`${apiBase}/api/provider-nodes/validate`, {
          method: 'POST',
          headers: { ...getHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({
            baseUrl,
            apiKey,
            type: isAnthropic ? 'anthropic-compatible' : 'openai-compatible'
          })
        });
        const resText = await res.text();
        let data = {};
        try { data = JSON.parse(resText); } catch {}
        
        if (data.valid) {
          valStatus.style.color = 'var(--lime)';
          valStatus.textContent = '✓ Endpoint reachable and responsive';
        } else {
          let errStr = '';
          if (typeof data.error === 'string') {
            errStr = data.error;
          } else if (data.error && typeof data.error.message === 'string') {
            errStr = data.error.message;
          } else if (typeof data.message === 'string') {
            errStr = data.message;
          } else if (typeof data.error === 'object') {
            errStr = JSON.stringify(data.error);
          }
          if (!errStr) errStr = resText || (res.status !== 200 ? `HTTP ${res.status}` : 'Unknown validation error');
          valStatus.style.color = 'var(--danger)';
          valStatus.textContent = `✕ Connection test failed: ${errStr}`;
        }
      } catch (err) {
        valStatus.style.color = 'var(--danger)';
        valStatus.textContent = `✕ Test error: ${err.message}`;
      } finally {
        valBtn.disabled = false;
      }
    };
  }

  form.onsubmit = async (e) => {
    e.preventDefault();
    const values = Object.fromEntries(new FormData(form).entries());
    submitBtn.disabled = true;
    submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving...';
    errEl.textContent = '';
    try {
      const payload = {
        name: values.name.trim(),
        prefix: values.prefix.trim(),
        baseUrl: values.baseUrl.trim(),
        type: isAnthropic ? 'anthropic-compatible' : 'openai-compatible'
      };
      if (!isAnthropic && values.apiType) {
        payload.apiType = values.apiType;
      }

      const url = isEdit ? `${apiBase}/api/provider-nodes/${existingNode.id}` : `${apiBase}/api/provider-nodes`;
      const method = isEdit ? 'PUT' : 'POST';

      const res = await fetch(url, {
        method,
        headers: { ...getHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const resText = await res.text();
      let data = {};
      try { data = JSON.parse(resText); } catch {}
      if (!res.ok) throw new Error(data.error || resText || `${res.status} ${res.statusText}`);

      showToast(isEdit ? 'Compatible node updated!' : 'Compatible node created!', 'success');
      cleanup();
      await renderView('providers');
    } catch (err) {
      errEl.textContent = err.message;
    } finally {
      submitBtn.disabled = false;
      submitBtn.innerHTML = isEdit ? 'Save Changes' : 'Create Node';
    }
  };
}
// ─────────────────────────────────────────────────────────────
// 5. OTHER PAGES (Combos, Keys, Usage, Logs, Pools, Tools, Chat)
// ─────────────────────────────────────────────────────────────
function parseComboModels(raw) {
  if (!raw) return [];
  if (Array.isArray(raw)) return raw;
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [String(raw)];
  } catch {
    return [String(raw)];
  }
}

function renderCombos(payload) {
  const rows = payload.combos || [];
  if (!rows.length) return emptySurface('No route combos configured. Click "+ Add connection" or "+ Create combo" to compose a fallback or round-robin route.');
  return `
    <div class="data-table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>Strategy</th>
            <th>Combo Name</th>
            <th>Models Pipeline Sequence</th>
            <th class="table-cell-actions">Actions</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map((item) => {
            const models = parseComboModels(item.models);
            const strategyClass = item.strategy === 'fallback' ? 'active' : item.strategy === 'round-robin' ? 'purple' : 'active';
            return `
              <tr>
                <td><span class="table-badge ${strategyClass}">${escapeHtml((item.strategy || 'fallback').toUpperCase())}</span></td>
                <td><strong style="color:var(--text-bright); font-size:12px;">${escapeHtml(item.name)}</strong></td>
                <td>
                  <div style="display:flex; align-items:center; flex-wrap:wrap; gap:4px;">
                    ${models.map((m, idx) => `
                      <span style="display:inline-flex; align-items:center; gap:4px;">
                        <code class="model-id-code" style="font-size:10.5px;">${escapeHtml(m)}</code>
                        ${idx < models.length - 1 ? '<span style="color:var(--dim); font-size:11px;">&rarr;</span>' : ''}
                      </span>
                    `).join('')}
                  </div>
                </td>
                <td class="table-cell-actions">
                  <button class="secondary-button" data-edit-combo="${escapeHtml(item.id)}" style="font-size:9.5px; padding:3px 7px;">Edit Flow</button>
                  <button class="danger-button" data-delete="orchestrator" data-id="${escapeHtml(item.id)}">Delete</button>
                </td>
              </tr>
            `;
          }).join('')}
        </tbody>
      </table>
    </div>
  `;
}

function renderKeys(payload) {
  const rows = payload.keys || [];
  if (!rows.length) return emptySurface('No API keys configured');
  return `
    <div class="data-table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>Status</th>
            <th>Key Name</th>
            <th>Key Token</th>
            <th>Policy Restrictions</th>
            <th class="table-cell-actions">Actions</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map((item) => `
            <tr>
              <td>
                <span class="table-badge ${item.isActive === 1 ? 'active' : 'inactive'}">
                  ${item.isActive === 1 ? 'ACTIVE' : 'INACTIVE'}
                </span>
              </td>
              <td><strong>${escapeHtml(item.name || 'Unnamed key')}</strong></td>
              <td>
                <span class="table-cell-mono" style="display:inline-flex; align-items:center; gap:6px; color:#b3c5a0;">
                  <span>${escapeHtml(item.key || '***')}</span>
                  ${item.id ? `<button class="model-copy-btn" data-copy-key-id="${escapeHtml(item.id)}" title="Copy Full API Key">&boxbox;</button>` : ''}
                </span>
              </td>
              <td>
                <div style="font-size:10px; font-family:var(--mono); color:var(--muted); max-width:320px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                  ${escapeHtml(item.restrictions || 'No restrictions (unlimited)')}
                </div>
              </td>
              <td class="table-cell-actions">
                <button class="secondary-button" data-edit-key="${escapeHtml(item.id)}" style="font-size:9.5px; padding:3px 7px;">Edit Policy</button>
                <button class="danger-button" data-delete="keys" data-id="${escapeHtml(item.id)}">Delete</button>
              </td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    </div>
  `;
}

function renderSettings(payload) {
  if (!payload) return emptySurface('No settings loaded');
  const source = JSON.stringify(payload, null, 2);
  return `
    <div class="aliases-deck-container">
      <!-- 1. Database Backup & Restore Card (100% 9router Parity) -->
      <div class="card" style="padding:18px;">
        <div style="display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid var(--line); padding-bottom:8px; margin-bottom:14px; flex-wrap:wrap; gap:8px;">
          <div>
            <span class="kicker">MIGRATION &amp; DISASTER RECOVERY</span>
            <h3 style="font-size:14px; margin:2px 0 0;">Database Backup &amp; Restore</h3>
          </div>
          <span class="table-badge purple" style="font-size:8px;">SQLITE SNAPSHOT</span>
        </div>
        <p style="font-size:11.5px; color:#9db2c6; margin:0 0 14px; line-height:1.5;">
          Export an entire snapshot of your SQLite database (all accounts, provider tokens, API keys, proxy pools, combos, and custom models) into a single backup JSON file, or restore from a previous backup.
        </p>
        <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
          <button class="solid-button" id="btn-export-database" type="button" style="font-size:11px; padding:7px 14px;">
            ⬇️ Export Full Database Backup
          </button>
          <label class="secondary-button" for="import-database-file" style="font-size:11px; padding:7px 14px; cursor:pointer; display:inline-flex; align-items:center; gap:6px;">
            ⬆️ Restore Database Backup
          </label>
          <input id="import-database-file" type="file" accept="application/json" hidden />
          <span id="db-backup-status" style="font-size:11px; font-family:var(--mono);"></span>
        </div>
      </div>

      <!-- 2. System Settings & Toggles Card -->
      <div class="card settings-surface" style="padding:18px;">
        <div class="card-top" style="border-bottom:1px solid var(--line); padding-bottom:8px; margin-bottom:14px;">
          <div>
            <span class="kicker">SYSTEM RUNTIME</span>
            <h3 style="font-size:14px; margin:2px 0 0;">Optimization &amp; Token Savers</h3>
          </div>
          <div class="settings-actions">
            <button class="cancel-button" id="export-settings" type="button" style="font-size:10px;">Export Settings JSON</button>
            <label class="cancel-button import-label" for="import-settings" style="font-size:10px; cursor:pointer;">Import Settings JSON</label>
            <input id="import-settings" type="file" accept="application/json" hidden />
          </div>
        </div>
        <div class="settings-toggles">
          <label><input type="checkbox" data-setting="rtkEnabled" ${payload.rtkEnabled ? 'checked' : ''} /> RTK compression</label>
          <label><input type="checkbox" data-setting="cavemanEnabled" ${payload.cavemanEnabled ? 'checked' : ''} /> Caveman mode</label>
          <label><input type="checkbox" data-setting="ponytailEnabled" ${payload.ponytailEnabled ? 'checked' : ''} /> Ponytail mode</label>
        </div>
        <textarea id="settings-json" rows="10" style="margin-top:10px;">${escapeHtml(source)}</textarea>
        <div class="form-actions" style="margin-top:10px;">
          <button class="solid-button" id="save-settings" type="button">Save Settings</button>
        </div>
        <p id="settings-result" class="result-line"></p>
      </div>

      <!-- 3. Dashboard Password Security Card -->
      <div class="card" style="padding:18px;">
        <div style="display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid var(--line); padding-bottom:8px; margin-bottom:14px;">
          <div>
            <span class="kicker">DASHBOARD SECURITY</span>
            <h3 style="font-size:14px; margin:2px 0 0;">Change Dashboard Password</h3>
          </div>
          <span class="table-badge active" style="font-size:8px;">ENCRYPTED SHA256/SALT</span>
        </div>
        <form id="change-password-form" style="display:grid; gap:10px; max-width:480px;">
          <label style="font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Current Password
            <input type="password" name="currentPassword" placeholder="Current dashboard password" required style="background:#05070a; border:1px solid var(--line); padding:7px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
          </label>
          <label style="font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            New Password
            <input type="password" name="newPassword" placeholder="Minimum 4 characters" required minlength="4" style="background:#05070a; border:1px solid var(--line); padding:7px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
          </label>
          <div style="display:flex; align-items:center; gap:10px; margin-top:4px;">
            <button class="solid-button" type="submit" id="btn-change-password-submit">Update Password</button>
            <span id="change-password-status" style="font-size:11px; font-family:var(--mono);"></span>
          </div>
        </form>
      </div>
    </div>
  `;
}

let aliasSearchQuery = '';
let aliasProviderFilter = 'all';
let aliasCurrentPage = 1;
let aliasPageSize = 25;
let cachedAliasesPayload = { aliases: {} };

function renderAliases(payload) {
  if (payload && payload.aliases) {
    cachedAliasesPayload = payload;
  }
  const allEntries = Object.entries(cachedAliasesPayload.aliases || {});
  if (!allEntries.length) return emptySurface('No model aliases configured. Click "+ Create alias" to map a client model name.');

  // Extract unique provider filters from target strings
  const providerCounts = { all: allEntries.length };
  allEntries.forEach(([alias, target]) => {
    let prov = 'custom';
    if (target.includes('/')) {
      prov = target.split('/')[0].toLowerCase();
    }
    providerCounts[prov] = (providerCounts[prov] || 0) + 1;
  });

  // Apply filters & search query
  const q = aliasSearchQuery.trim().toLowerCase();
  const filtered = allEntries.filter(([alias, target]) => {
    if (aliasProviderFilter !== 'all') {
      let prov = 'custom';
      if (target.includes('/')) {
        prov = target.split('/')[0].toLowerCase();
      }
      if (prov !== aliasProviderFilter) return false;
    }
    if (q) {
      return alias.toLowerCase().includes(q) || target.toLowerCase().includes(q);
    }
    return true;
  });

  const totalPages = Math.ceil(filtered.length / aliasPageSize) || 1;
  if (aliasCurrentPage > totalPages) aliasCurrentPage = totalPages;
  if (aliasCurrentPage < 1) aliasCurrentPage = 1;

  const startIdx = (aliasCurrentPage - 1) * aliasPageSize;
  const endIdx = Math.min(startIdx + aliasPageSize, filtered.length);
  const pageItems = filtered.slice(startIdx, endIdx);

  const providerTabs = Object.keys(providerCounts)
    .sort((a, b) => (providerCounts[b] || 0) - (providerCounts[a] || 0))
    .slice(0, 12);

  return `
    <div class="aliases-deck-container">
      <!-- 1. Toolbar Card -->
      <div class="aliases-toolbar-card">
        <div class="aliases-toolbar-top">
          <div class="aliases-search-box">
            <span class="material-symbols-outlined search-icon">search</span>
            <input type="text" id="alias-search-input" value="${escapeHtml(aliasSearchQuery)}" placeholder="Search alias (e.g. gpt-4o) or target model..." />
          </div>

          <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
            <span class="table-badge purple" style="font-size:9.5px; padding:4px 8px;">
              ${filtered.length} / ${allEntries.length} ALIASES
            </span>
            ${allEntries.length > 0 ? `
              <button class="danger-button" id="btn-clear-all-aliases" type="button" style="font-size:11px; padding:6px 12px; display:inline-flex; align-items:center; gap:4px;">
                <span class="material-symbols-outlined" style="font-size:13px;">delete_sweep</span> Delete All (${allEntries.length})
              </button>
            ` : ''}
            <button class="solid-button" id="btn-open-create-alias" type="button" style="font-size:11px; padding:6px 12px;">
              <span>+</span> Create New Alias
            </button>
          </div>
        </div>

        <!-- Filter Chips -->
        <div class="aliases-filter-tabs">
          <button type="button" class="alias-filter-chip ${aliasProviderFilter === 'all' ? 'active' : ''}" data-filter-prov="all">
            All (${allEntries.length})
          </button>
          ${providerTabs.filter(p => p !== 'all').map((p) => `
            <button type="button" class="alias-filter-chip ${aliasProviderFilter === p ? 'active' : ''}" data-filter-prov="${escapeHtml(p)}">
              ${escapeHtml(p.toUpperCase())} (${providerCounts[p] || 0})
            </button>
          `).join('')}
        </div>
      </div>

      <!-- 2. Paginated Aliases Table -->
      <div class="data-table-container card" style="padding:0; overflow:hidden;">
        <table class="data-table">
          <thead>
            <tr>
              <th style="width:100px;">Status</th>
              <th style="width:30%;">Client Model Name (Alias)</th>
              <th style="width:40%;">Target Upstream Route</th>
              <th class="table-cell-actions" style="width:20%;">Actions</th>
            </tr>
          </thead>
          <tbody>
            ${pageItems.length === 0 ? `
              <tr>
                <td colspan="4" style="text-align:center; color:var(--muted); padding:30px 14px;">
                  No aliases match the search query "${escapeHtml(aliasSearchQuery)}".
                </td>
              </tr>
            ` : pageItems.map(([alias, target]) => {
              let targetProv = 'gateway';
              if (target.includes('/')) {
                targetProv = target.split('/')[0];
              }
              return `
                <tr id="alias-row-${escapeHtml(alias)}">
                  <td><span class="table-badge active" style="font-size:7.5px;">ACTIVE</span></td>
                  <td>
                    <div style="display:flex; align-items:center; gap:6px;">
                      <code class="model-id-code" style="color:var(--text-bright); font-size:11px; font-weight:600;">${escapeHtml(alias)}</code>
                      <button class="model-copy-btn" data-copy-text="${escapeHtml(alias)}" title="Copy alias identifier" style="font-size:11px; padding:2px;">&boxbox;</button>
                    </div>
                  </td>
                  <td>
                    <div class="alias-mapping-cell">
                      <span class="alias-route-arrow">&rarr;</span>
                      <span class="table-badge" style="font-size:8px; padding:2px 5px; background:rgba(255,255,255,0.05); color:var(--muted); text-transform:uppercase;">${escapeHtml(targetProv)}</span>
                      <code class="model-id-code" style="color:var(--lime); font-size:11px;">${escapeHtml(target)}</code>
                    </div>
                  </td>
                  <td class="table-cell-actions">
                    <button class="model-test-btn" data-test-alias="${escapeHtml(alias)}" style="font-size:9.5px; padding:3px 7px;">Test Live</button>
                    <button class="secondary-button" data-edit-alias="${escapeHtml(alias)}" data-target="${escapeHtml(target)}" style="font-size:9.5px; padding:3px 7px;">Edit</button>
                    <button class="danger-button" data-delete-alias="${escapeHtml(alias)}" style="font-size:9.5px; padding:3px 7px;">Delete</button>
                  </td>
                </tr>
              `;
            }).join('')}
          </tbody>
        </table>

        <!-- 3. Pagination Footer -->
        <div class="aliases-pagination-bar">
          <div>
            Showing <strong>${filtered.length > 0 ? startIdx + 1 : 0}&ndash;${endIdx}</strong> of <strong>${filtered.length}</strong> mappings
          </div>

          <div class="aliases-pagination-controls">
            <label style="display:flex; align-items:center; gap:4px; font-size:10px; margin-right:8px;">
              Per page:
              <select id="alias-page-size-select" style="background:#05070a; border:1px solid var(--line); color:var(--text); font:10px var(--mono); padding:2px 4px; border-radius:3px;">
                <option value="15" ${aliasPageSize === 15 ? 'selected' : ''}>15</option>
                <option value="25" ${aliasPageSize === 25 ? 'selected' : ''}>25</option>
                <option value="50" ${aliasPageSize === 50 ? 'selected' : ''}>50</option>
                <option value="100" ${aliasPageSize === 100 ? 'selected' : ''}>100</option>
              </select>
            </label>

            <button type="button" class="alias-page-btn" id="btn-alias-prev" ${aliasCurrentPage <= 1 ? 'disabled' : ''}>&larr; Prev</button>
            <span style="padding:0 4px;">Page <strong>${aliasCurrentPage}</strong> / ${totalPages}</span>
            <button type="button" class="alias-page-btn" id="btn-alias-next" ${aliasCurrentPage >= totalPages ? 'disabled' : ''}>Next &rarr;</button>
          </div>
        </div>
      </div>
    </div>
  `;
}

let usageRecentPage = 1;
const usageRecentPageSize = 10;
let cachedUsagePayload = {};

function renderSparkline(points = [], color = '#c8ff63') {
  if (!points || points.length < 2) {
    return `
      <svg class="metric-sparkline-svg" viewBox="0 0 100 24" preserveAspectRatio="none">
        <path d="M 0 20 Q 50 18 100 16" fill="none" stroke="${color}" stroke-width="1.5" stroke-linecap="round" opacity="0.4" />
      </svg>
    `;
  }

  const min = Math.min(...points);
  const max = Math.max(...points);
  const range = max - min || 1;
  const width = 100;
  const height = 24;
  const padY = 3;

  const coords = points.map((p, i) => {
    const x = (i / (points.length - 1)) * width;
    const y = height - padY - ((p - min) / range) * (height - padY * 2);
    return [x, y];
  });

  const pathD = coords.map((c, i) => `${i === 0 ? 'M' : 'L'} ${c[0].toFixed(1)} ${c[1].toFixed(1)}`).join(' ');
  const areaD = `${pathD} L ${width} ${height} L 0 ${height} Z`;
  const gradId = `grad-${Math.random().toString(36).slice(2, 7)}`;

  return `
    <svg class="metric-sparkline-svg" viewBox="0 0 ${width} ${height}" preserveAspectRatio="none">
      <defs>
        <linearGradient id="${gradId}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="${color}" stop-opacity="0.3" />
          <stop offset="100%" stop-color="${color}" stop-opacity="0.0" />
        </linearGradient>
      </defs>
      <path d="${areaD}" fill="url(#${gradId})" />
      <path d="${pathD}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  `;
}

function renderUsage(payload) {
  if (payload) {
    cachedUsagePayload = payload;
  }
  const p = cachedUsagePayload || {};
  const active = Array.isArray(p.activeRequests) ? p.activeRequests : [];
  const recent = Array.isArray(p.recentRequests) ? p.recentRequests : [];
  // Extract prompt & completion tokens from either Go engine or Next.js engine
  const promptTokens = Number(p.promptTokens ?? p.totalPromptTokens ?? 0);
  const completionTokens = Number(p.completionTokens ?? p.totalCompletionTokens ?? 0);
  const totalTokens = Number(p.totalTokens ?? (promptTokens + completionTokens) ?? 0);
  const totalCost = Number(p.totalCost || 0);

  let totalRequests = Number(p.totalRequests || 0);
  if (totalRequests === 0 && p.byProvider && typeof p.byProvider === 'object') {
    totalRequests = Object.values(p.byProvider).reduce((acc, x) => acc + (Number(x?.requests) || 0), 0);
  }
  if (totalRequests === 0 && recent.length > 0) {
    totalRequests = recent.length;
  }

  // Extract daily breakdown array from either p.daily or p.byModel
  let daily = Array.isArray(p.daily) ? p.daily : [];
  if (daily.length === 0 && p.byModel && typeof p.byModel === 'object') {
    const dayMap = new Map();
    Object.values(p.byModel).forEach((m) => {
      const d = m.lastUsed || '';
      if (d) {
        if (!dayMap.has(d)) dayMap.set(d, { date: d, requests: 0, tokens: 0, cost: 0 });
        const entry = dayMap.get(d);
        entry.requests += Number(m.requests) || 0;
        entry.tokens += (Number(m.promptTokens) || 0) + (Number(m.completionTokens) || 0);
        entry.cost += Number(m.cost) || 0;
      }
    });
    daily = Array.from(dayMap.values()).sort((a, b) => b.date.localeCompare(a.date));
  }

  // Generate sparkline trend data points
  const requestPoints = daily.map(d => Number(d.requests) || 0).reverse();
  const tokenPoints = daily.map(d => Number(d.tokens || (Number(d.promptTokens || 0) + Number(d.completionTokens || 0))) || 0).reverse();
  const costPoints = daily.map(d => Number(d.cost) || 0).reverse();

  // Paginate Recent Request History
  const recentList = recent.length ? recent : active;
  const totalRecentPages = Math.ceil(recentList.length / usageRecentPageSize) || 1;
  if (usageRecentPage > totalRecentPages) usageRecentPage = totalRecentPages;
  if (usageRecentPage < 1) usageRecentPage = 1;

  const startRecentIdx = (usageRecentPage - 1) * usageRecentPageSize;
  const endRecentIdx = Math.min(startRecentIdx + usageRecentPageSize, recentList.length);
  const pageRecentItems = recentList.slice(startRecentIdx, endRecentIdx);

  return `
    <form class="filter-bar" id="usage-filters">
      <label>Window
        <select name="days">
          <option value="all" selected>All time</option>
          <option value="90">Last 90 days</option>
          <option value="30">Last 30 days</option>
          <option value="7">Last 7 days</option>
        </select>
      </label>
      <label>Provider<input name="provider" placeholder="Optional filter (e.g. antigravity)..." /></label>
      <label>Model<input name="model" placeholder="Optional filter (e.g. gemini-3.7)..." /></label>
      <button class="secondary-button" type="submit">Apply filters</button>
    </form>

    <div class="usage-summary">
      <!-- 1. TOTAL REQUESTS CARD -->
      <div class="usage-metric-card requests">
        <div class="usage-metric-header">
          <span class="usage-metric-kicker">TOTAL REQUESTS</span>
          <span class="stat-chip-pill cyan">
            <span class="material-symbols-outlined" style="font-size:11px;">data_usage</span>
            Traffic
          </span>
        </div>
        <div class="usage-metric-body">
          <div class="usage-metric-val" id="usage-total-requests">${totalRequests.toLocaleString()}</div>
          <div class="usage-metric-spark" id="usage-spark-requests">${renderSparkline(requestPoints, '#38bdf8')}</div>
        </div>
        <div class="usage-metric-footer">
          <span>In selected window</span>
          <span style="color:#38bdf8;">${recentList.length} stream traces</span>
        </div>
      </div>

      <!-- 2. TOKEN USAGE CARD -->
      <div class="usage-metric-card tokens">
        <div class="usage-metric-header">
          <span class="usage-metric-kicker">TOKEN USAGE</span>
          <span class="stat-chip-pill emerald">
            <span class="material-symbols-outlined" style="font-size:11px;">memory</span>
            Compute
          </span>
        </div>
        <div class="usage-metric-body">
          <div class="usage-metric-val" id="usage-total-tokens" title="${totalTokens.toLocaleString('en-US')} tokens">${formatTokenCount(totalTokens)}</div>
          <div class="usage-metric-spark" id="usage-spark-tokens">${renderSparkline(tokenPoints, '#c8ff63')}</div>
        </div>
        <div class="usage-metric-footer">
          <span>Prompt + Completion</span>
          <span style="color:#c8ff63;">${promptTokens > 0 ? `${Math.round(promptTokens/1000)}k in / ${Math.round(completionTokens/1000)}k out` : 'Zero alloc'}</span>
        </div>
      </div>

      <!-- 3. ESTIMATED COST CARD -->
      <div class="usage-metric-card cost">
        <div class="usage-metric-header">
          <span class="usage-metric-kicker">ESTIMATED COST</span>
          <span class="stat-chip-pill purple">
            <span class="material-symbols-outlined" style="font-size:11px;">payments</span>
            Arbitrage
          </span>
        </div>
        <div class="usage-metric-body">
          <div class="usage-metric-val" id="usage-total-cost">${totalCost > 0 ? `$${totalCost.toFixed(4)}` : '$0.00'}</div>
          <div class="usage-metric-spark" id="usage-spark-cost">${renderSparkline(costPoints, '#c084fc')}</div>
        </div>
        <div class="usage-metric-footer">
          <span>Based on provider ledger</span>
          <span style="color:#c084fc;">Real-time balance</span>
        </div>
      </div>
    </div>


    <!-- Recent Requests Activity Table Card with Pagination -->
    <div class="card" style="padding:14px;">
      <div class="card-top" style="margin-bottom:10px;">
        <div>
          <span class="kicker">TELEMETRY STREAM</span>
          <h3 style="font-size:13.5px; margin:2px 0 0;">Recent Request Activity</h3>
        </div>
        <span style="font-size:9.5px; font-family:var(--mono); color:var(--dim);" id="usage-recent-count-badge">${recentList.length} RECENT</span>
      </div>
      <div class="data-table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th style="width:75px;">Time</th>
              <th>Model</th>
              <th>Provider &amp; Account</th>
              <th>Outbound Proxy</th>
              <th style="width:80px;">Tokens</th>
              <th style="text-align: right; width:70px;">Status</th>
            </tr>
          </thead>
          <tbody id="usage-recent-tbody">
              ${pageRecentItems.length ? pageRecentItems.map((item) => {
                const timeStr = item.timestamp ? new Date(item.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '--:--:--';
                const dateStr = item.timestamp ? new Date(item.timestamp).toLocaleDateString([], { month: 'short', day: 'numeric' }) : '';
                const toks = Number((item.promptTokens || 0) + (item.completionTokens || 0));
                const isErr = item.status === 'error' || String(item.status).startsWith('4') || String(item.status).startsWith('5');
                const pName = (item.provider || 'gateway').toLowerCase();
                const accName = item.account || item.connectionId || '--';
                const proxyName = item.proxy || 'Direct';
                const isRelay = proxyName !== 'Direct' && proxyName !== '--';
                return `
                  <tr>
                    <td>
                      <div style="font-family:var(--mono); line-height:1.2;">
                        <span style="font-size:10px; color:var(--text-bright); font-weight:500;">${escapeHtml(timeStr)}</span>
                        <small style="display:block; font-size:8px; color:var(--muted);">${escapeHtml(dateStr)}</small>
                      </div>
                    </td>
                    <td><code class="model-id-code" style="font-size:10.5px;">${escapeHtml(item.model || '--')}</code></td>
                    <td>
                      <div style="line-height:1.25;">
                        <strong style="color:var(--text-bright); font-size:11px;">${escapeHtml(pName)}</strong>
                        <small style="display:block; font-size:8.5px; font-family:var(--mono); color:var(--muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; max-width:180px;">${escapeHtml(accName)}</small>
                      </div>
                    </td>
                    <td>
                      <div style="line-height:1.25;">
                        <span class="table-badge ${isRelay ? 'purple' : ''}" style="font-size:8px; padding:1px 5px;">${escapeHtml(proxyName)}</span>
                        ${item.strategy ? `<small style="display:block; font-size:7.5px; font-family:var(--mono); color:#71717a; text-transform:uppercase; margin-top:2px;">${escapeHtml(item.strategy)}</small>` : ''}
                      </div>
                    </td>
                    <td><span class="table-cell-mono" style="font-size:10px; color:var(--text);">${toks > 0 ? `${toks.toLocaleString()}t` : '--'}</span></td>
                    <td style="text-align: right;"><span class="table-badge ${isErr ? 'inactive' : 'active'}" style="font-size:7.5px;">${escapeHtml(item.status || item.count || '200')}</span></td>
                  </tr>
                `;
              }).join('') : '<tr><td colspan="6" style="text-align:center; color:var(--dim); padding:20px;">No requests recorded yet.</td></tr>'}
            </tbody>
          </table>
        </div>
        <!-- Recent Requests Pagination Footer -->
        <div class="aliases-pagination-bar" style="padding:6px 12px; margin-top:8px;">
          <span style="font-size:9.5px;">Showing <strong>${recentList.length > 0 ? startRecentIdx + 1 : 0}&ndash;${endRecentIdx}</strong> of <strong>${recentList.length}</strong></span>
          <div class="aliases-pagination-controls">
            <button type="button" class="alias-page-btn" id="btn-usage-recent-prev" ${usageRecentPage <= 1 ? 'disabled' : ''} style="padding:2px 6px; font-size:9px;">&larr; Prev</button>
            <span style="font-size:9.5px; padding:0 3px;">Page <strong>${usageRecentPage}</strong> / ${totalRecentPages}</span>
            <button type="button" class="alias-page-btn" id="btn-usage-recent-next" ${usageRecentPage >= totalRecentPages ? 'disabled' : ''} style="padding:2px 6px; font-size:9px;">Next &rarr;</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 4. Raw Unredacted Audit Logs (50MB Rolling Files) Section -->
    <div class="card" style="padding:16px; margin-top:14px;">
      <div class="card-top" style="margin-bottom:12px; display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:10px;">
        <div>
          <div style="display:flex; align-items:center; gap:8px;">
            <span class="kicker" style="color:var(--lime); margin:0;">AUDIT &amp; RAW TRANSACTION VAULT</span>
            <span class="table-badge active" style="font-size:7.5px; padding:1px 5px;">50MB AUTO-ROLLING</span>
          </div>
          <h3 style="font-size:14px; margin:3px 0 0; font-weight:700;">Full Unredacted Payload Archive</h3>
          <p style="font-size:11.5px; color:var(--muted); margin:2px 0 0;">Stores 100% full unredacted client requests, upstream provider payloads, raw SSE streams, and client responses.</p>
        </div>
        <button class="secondary-button" id="btn-refresh-audit-files" type="button" style="font-size:10.5px; padding:6px 12px; display:inline-flex; align-items:center; gap:4px;">
          <span class="material-symbols-outlined" style="font-size:14px;">refresh</span> Refresh Files
        </button>
      </div>

      <div id="audit-files-table-slot" style="min-height:80px;">
        <div style="text-align:center; padding:20px; color:var(--muted); font-size:11.5px;">
          <span class="spinner-icon"></span> Loading audit log file catalog...
        </div>
      </div>
    </div>
  `;
}

function renderLogs(payload = {}) {
  const recent = Array.isArray(payload.recentRequests) ? payload.recentRequests : [];
  const initialCount = recent.length;

  return `
    <div class="console-view-container">
      <!-- 1. Header Toolbar Bar -->
      <div class="console-header-card">
        <div style="display:flex; align-items:center; gap:10px;">
          <div class="console-status-indicator">
            <span class="pulse-dot"></span>
            <span id="console-stream-status-text">Live Request Stream</span>
          </div>
          <span style="font-size:11px; font-family:var(--mono); color:#4a5c6d; background:rgba(255,255,255,0.04); padding:2px 7px; border-radius:10px;" id="console-buffer-count">${initialCount} reqs</span>
        </div>

        <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap;">
          <button class="cancel-button" id="btn-console-pause" style="font-size:11px; padding:4px 10px; color:#e2e8f0; background:#121824; border:1px solid rgba(255,255,255,0.08); border-radius:6px;">
            <span style="color:#f59e0b; font-size:10px; margin-right:4px;">⏸</span> Pause Stream
          </button>
          <button class="cancel-button" id="btn-console-clear" style="font-size:11px; padding:4px 10px; color:#94a3b8; background:none; border:none; display:flex; align-items:center; gap:4px;">
            <span class="material-symbols-outlined" style="font-size:14px;">delete</span> Clear
          </button>
        </div>
      </div>

      <!-- 2. Search & Multi-Segment Filter Bar -->
      <div class="console-filter-bar">
        <div class="console-search-wrapper">
          <span style="color:#4a5c6d; font-size:13px;">🔍</span>
          <input class="console-search-input" id="console-search-input" placeholder="Search by ID, model, status (e.g. 200, claude, post)..." />
        </div>

        <div style="display:flex; align-items:center; gap:10px;">
          <div class="console-tab-group" id="console-status-filter">
            <button type="button" class="console-tab-btn active" data-filter="all">All</button>
            <button type="button" class="console-tab-btn" data-filter="2xx">2xx OK</button>
            <button type="button" class="console-tab-btn" data-filter="4xx/5xx">4xx/5xx</button>
          </div>

          <select class="console-prov-select" id="console-provider-filter">
            <option value="all" selected>All Providers</option>
            <option value="codex">Codex</option>
            <option value="antigravity">Antigravity</option>
            <option value="gemini">Google Gemini</option>
            <option value="openai">OpenAI</option>
            <option value="mistral">Mistral</option>
            <option value="custom">Custom Nodes</option>
          </select>
        </div>
      </div>

      <!-- 3. Active In-Flight Requests Slot -->
      <div id="console-active-container" class="console-active-grid" style="display:none; padding:10px 20px;"></div>

      <!-- 4. Cyber Live Request Stream Table -->
      <div class="console-table-card">
        <div style="max-height: 560px; overflow-y: auto;" id="console-scroll-container">
          <table class="console-stream-table">
            <thead>
              <tr>
                <th style="width: 80px;">STATUS</th>
                <th style="width: 250px;">METHOD &amp; ID</th>
                <th>MODEL &amp; PROVIDER</th>
                <th style="width: 90px; text-align: right;">TOKENS</th>
                <th style="width: 85px; text-align: right;">LATENCY</th>
                <th style="width: 80px; text-align: right;">SAVINGS</th>
                <th style="width: 195px; text-align: right;">TIME</th>
              </tr>
            </thead>
            <tbody id="console-terminal-body">
              ${recent.length === 0 ? `
                <tr id="console-empty-row">
                  <td colspan="7" style="text-align: center; color: var(--muted); padding: 48px 14px; font-style: italic;">
                    Waiting for live requests... Run a client completion to see real-time streaming traffic.
                  </td>
                </tr>
              ` : recent.map((req, idx) => {
                const timeStr = req.timestamp ? new Date(req.timestamp).toISOString().replace('T', ' ').slice(0, 23) + 'Z' : new Date().toISOString().replace('T', ' ').slice(0, 23) + 'Z';
                const isErr = req.status === 'error' || String(req.status).startsWith('4') || String(req.status).startsWith('5');
                const statusCode = req.status || 200;
                const statusDotColor = isErr ? '#ef4444' : '#22c55e';
                const totalTokens = (req.promptTokens || 0) + (req.completionTokens || 0);
                const latencySec = req.durationMs ? (req.durationMs / 1000).toFixed(2) + 's' : (req.latency ? req.latency : '1.42s');
                const latNum = parseFloat(latencySec) || 0;
                const latClass = latNum > 15 ? 'slow' : (latNum > 4 ? 'med' : 'fast');
                const reqId = req.id || `${Date.now() - idx * 1000}-${req.model || 'chat'}`;
                const provName = (req.provider || 'gateway').toLowerCase();
                const costSavings = req.savings ? `$${req.savings.toFixed(2)}` : '$0.00';

                return `
                  <tr class="console-row" data-status="${escapeHtml(String(statusCode))}" data-provider="${escapeHtml(provName)}" data-query="${escapeHtml(`${reqId} ${provName} ${req.model || ''} ${statusCode}`.toLowerCase())}" data-req='${escapeHtml(JSON.stringify(req))}'>
                    <td>
                      <span style="display:inline-flex; align-items:center; gap:6px; font-weight:700; color:${statusDotColor};">
                        <span style="font-size:9px;">●</span> ${escapeHtml(String(statusCode))}
                      </span>
                    </td>
                    <td>
                      <span class="method-tag post">POST</span>
                      <span style="color:#94a3b8; font-size:11px;">${escapeHtml(reqId)}</span>
                    </td>
                    <td>
                      <div style="display:flex; align-items:center; gap:8px;">
                        <span class="prov-pill ${escapeHtml(provName.startsWith('openai-compatible') ? 'custom' : provName)}">
                          ${escapeHtml(provName)}
                        </span>
                        <strong style="color:var(--text-bright); font-size:11.5px;">${escapeHtml(req.model || 'model')}</strong>
                      </div>
                    </td>
                    <td style="text-align: right; color:#e2e8f0;">
                      ${totalTokens > 0 ? `${totalTokens.toLocaleString()} tok` : '--'}
                    </td>
                    <td style="text-align: right;">
                      <span class="latency-badge ${latClass}">${escapeHtml(latencySec)}</span>
                    </td>
                    <td style="text-align: right; color:#facc15;">
                      ${escapeHtml(costSavings)}
                    </td>
                    <td style="text-align: right; color:#64748b; font-size:10px;">
                      ${escapeHtml(timeStr)}
                    </td>
                  </tr>
                `;
              }).join('')}
            </tbody>
          </table>
        </div>
      </div>

      <!-- 5. Footer Info Bar -->
      <div class="console-footer-bar">
        <span>Click row to open Payload Inspector drawer</span>
        <span id="console-footer-count">Showing <strong>${initialCount}</strong> of ${initialCount}</span>
      </div>
    </div>
  `;
}

let poolSearchQuery = '';
let poolTypeFilter = 'all';
let poolStatusFilter = 'all';
let poolCurrentPage = 1;
let poolPageSize = 15;
let cachedPoolsPayload = { proxyPools: [] };

function renderPools(payload) {
  if (payload) cachedPoolsPayload = payload;
  const allRows = cachedPoolsPayload.proxyPools || [];

  // Type counts
  const typeCounts = { all: allRows.length, vercel: 0, cloudflare: 0, deno: 0, http: 0 };
  allRows.forEach((item) => {
    let d = {};
    try { d = typeof item.data === 'string' ? JSON.parse(item.data) : (item.data || {}); } catch {}
    const t = (item.type || d.type || 'http').toLowerCase();
    if (typeCounts[t] !== undefined) typeCounts[t]++;
    else typeCounts[t] = 1;
  });

  // Filter by search, type, and status
  const q = poolSearchQuery.trim().toLowerCase();
  const filtered = allRows.filter((item) => {
    let d = {};
    try { d = typeof item.data === 'string' ? JSON.parse(item.data) : (item.data || {}); } catch {}
    const name = (item.name || d.name || item.id || '').toLowerCase();
    const url = (item.proxyUrl || d.proxyUrl || item.url || d.url || item.data || '').toLowerCase();
    const pType = (item.type || d.type || 'http').toLowerCase();
    const isActive = item.isActive === 1 || item.isActive === true || item.isActive === '1';
    const testStatus = (item.testStatus || d.testStatus || (isActive ? 'active' : 'untested')).toLowerCase();

    if (q && !name.includes(q) && !url.includes(q) && !item.id.toLowerCase().includes(q)) return false;
    if (poolTypeFilter !== 'all' && pType !== poolTypeFilter) return false;
    if (poolStatusFilter === 'active' && !isActive) return false;
    if (poolStatusFilter === 'disabled' && isActive) return false;
    if (poolStatusFilter === 'passed' && testStatus !== 'active' && testStatus !== 'pass' && testStatus !== 'passed') return false;
    if (poolStatusFilter === 'error' && testStatus !== 'error' && testStatus !== 'failed') return false;
    return true;
  });

  const totalPages = Math.ceil(filtered.length / poolPageSize) || 1;
  if (poolCurrentPage > totalPages) poolCurrentPage = totalPages;
  if (poolCurrentPage < 1) poolCurrentPage = 1;

  const startIdx = (poolCurrentPage - 1) * poolPageSize;
  const endIdx = Math.min(startIdx + poolPageSize, filtered.length);
  const pageRows = filtered.slice(startIdx, endIdx);

  const table = filtered.length ? `
    <div class="data-table-container card" style="padding:0; overflow:hidden; margin-top:10px;">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width:90px;">Status</th>
            <th>Pool Name</th>
            <th style="width:90px;">Type</th>
            <th>Proxy / Relay URL</th>
            <th style="width:110px;">Test Status</th>
            <th class="table-cell-actions" style="width:150px; text-align:right;">Action</th>
          </tr>
        </thead>
        <tbody>
          ${pageRows.map((item) => {
            let d = {};
            try { d = typeof item.data === 'string' ? JSON.parse(item.data) : (item.data || {}); } catch {}
            const poolName = item.name || d.name || item.id;
            const proxyType = (item.type || d.type || 'http').toUpperCase();
            const url = item.proxyUrl || d.proxyUrl || item.url || d.url || item.data || '--';
            const isActive = item.isActive === 1 || item.isActive === true || item.isActive === '1';
            const testStatus = item.testStatus || d.testStatus || (isActive ? 'active' : 'untested');
            const isTestOk = testStatus === 'active' || testStatus === 'pass' || testStatus === 'passed';
            const isTestErr = testStatus === 'error' || testStatus === 'failed';
            return `
              <tr id="row-pool-${escapeHtml(item.id)}" data-pool-id="${escapeHtml(item.id)}">
                <td>
                  <span class="table-badge ${isActive ? 'active' : 'inactive'}" id="badge-active-${escapeHtml(item.id)}">
                    ${isActive ? 'ACTIVE' : 'DISABLED'}
                  </span>
                </td>
                <td><strong style="color:var(--text-bright);">${escapeHtml(poolName)}</strong></td>
                <td><span class="table-badge purple">${escapeHtml(proxyType)}</span></td>
                <td><code class="model-id-code" style="font-size:10.5px;">${escapeHtml(url)}</code></td>
                <td>
                  <span class="table-badge ${isTestOk ? 'active' : (isTestErr ? 'error' : 'inactive')}" id="badge-status-${escapeHtml(item.id)}" style="font-size:8px;">
                    ${isTestOk ? 'PASSED' : (isTestErr ? 'ERROR' : escapeHtml(testStatus.toUpperCase()))}
                  </span>
                </td>
                <td class="table-cell-actions" style="text-align:right;">
                  <button class="secondary-button" data-test-pool="${escapeHtml(item.id)}" id="btn-test-pool-${escapeHtml(item.id)}" style="font-size:9.5px; padding:3px 8px; margin-right:4px;">Test</button>
                  <button class="danger-button" data-delete="pools" data-id="${escapeHtml(item.id)}">Delete</button>
                </td>
              </tr>
            `;
          }).join('')}
        </tbody>
      </table>

      <!-- Pagination Footer -->
      <div class="aliases-pagination-bar" style="padding:8px 14px; background:#070a0f; border-top:1px solid var(--line);">
        <div style="font-size:11px; color:var(--muted);">
          Showing <strong>${filtered.length > 0 ? startIdx + 1 : 0}&ndash;${endIdx}</strong> of <strong>${filtered.length}</strong> pools ${filtered.length !== allRows.length ? `(filtered from ${allRows.length})` : ''}
        </div>

        <div class="aliases-pagination-controls" style="display:flex; align-items:center; gap:8px;">
          <label style="display:flex; align-items:center; gap:4px; font-size:10.5px; margin-right:8px; color:var(--muted);">
            Per page:
            <select id="pool-page-size-select" style="background:#05070a; border:1px solid var(--line); color:var(--text); font:10px var(--mono); padding:2px 6px; border-radius:4px;">
              <option value="15" ${poolPageSize === 15 ? 'selected' : ''}>15</option>
              <option value="25" ${poolPageSize === 25 ? 'selected' : ''}>25</option>
              <option value="50" ${poolPageSize === 50 ? 'selected' : ''}>50</option>
              <option value="100" ${poolPageSize === 100 ? 'selected' : ''}>100</option>
            </select>
          </label>

          <button type="button" class="alias-page-btn" id="btn-pool-prev" ${poolCurrentPage <= 1 ? 'disabled' : ''}>&larr; Prev</button>
          <span style="font-size:11px; font-family:var(--mono); padding:0 4px;">Page <strong>${poolCurrentPage}</strong> / ${totalPages}</span>
          <button type="button" class="alias-page-btn" id="btn-pool-next" ${poolCurrentPage >= totalPages ? 'disabled' : ''}>Next &rarr;</button>
        </div>
      </div>
    </div>
  ` : emptySurface('No proxy pools found matching your filter criteria.');

  return `
    <div class="card" style="margin-bottom:12px; padding:16px 20px; background:linear-gradient(135deg, rgba(200,255,99,0.04) 0%, rgba(13,18,25,0.7) 100%); border:1px solid rgba(200,255,99,0.18);">
      <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:12px;">
        <div>
          <div style="display:flex; align-items:center; gap:8px;">
            <span class="kicker" style="color:var(--lime); margin:0;">PROXY &amp; RELAY DEPLOYMENT</span>
            <span class="table-badge active" style="font-size:7.5px; padding:1px 5px;">ROTATION READY</span>
          </div>
          <h2 style="font-size:16px; margin:4px 0 2px; font-weight:700; color:var(--text-bright);">Outbound Proxy Pools</h2>
          <p style="font-size:11.5px; color:var(--muted); margin:0;">Deploy global serverless relays or add HTTP/SOCKS5 proxies to bypass IP throttling.</p>
        </div>

        <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
          ${allRows.length > 0 ? `
            <button class="solid-button" id="btn-test-all-pools" style="font-size:11px; padding:8px 14px; display:inline-flex; align-items:center; gap:6px; box-shadow:0 0 16px rgba(200,255,99,0.2);">
              <span class="material-symbols-outlined" style="font-size:15px;">health_and_safety</span>
              Health Check All (${allRows.length})
            </button>
          ` : ''}
          <div class="deploy-actions" style="display:flex; gap:6px;">
            <button data-deploy="custom" class="secondary-button" style="font-size:10.5px; padding:7px 11px;">+ Custom</button>
            <button data-deploy="cloudflare" class="secondary-button" style="font-size:10.5px; padding:7px 11px;">⚡ Cloudflare</button>
            <button data-deploy="deno" class="secondary-button" style="font-size:10.5px; padding:7px 11px;">🦕 Deno</button>
            <button data-deploy="vercel" class="secondary-button" style="font-size:10.5px; padding:7px 11px;">▲ Vercel</button>
          </div>
        </div>
      </div>
      <div id="deploy-form-slot" style="grid-column:1/-1; width:100%; margin-top:8px;"></div>
    </div>

    <!-- Search, Filter & Live Progress Card -->
    <div class="aliases-toolbar-card" style="margin-bottom:12px; padding:12px 16px; background:#080c12; border:1px solid var(--line); border-radius:8px;">
      <div class="aliases-toolbar-top" style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px;">
        <div class="aliases-search-box" style="flex:1; min-width:260px; max-width:440px;">
          <span class="material-symbols-outlined search-icon" style="font-size:16px;">search</span>
          <input type="text" id="pool-search-input" value="${escapeHtml(poolSearchQuery)}" placeholder="Search proxy name, URL host, or ID..." style="font-size:11px; padding:7px 10px 7px 30px;" />
        </div>

        <div style="display:flex; align-items:center; gap:6px;">
          <span class="table-badge purple" style="font-size:9.5px; padding:3px 8px;">
            ${filtered.length} / ${allRows.length} POOLS
          </span>
        </div>
      </div>

      <!-- Filter Tabs -->
      <div class="aliases-filter-tabs" style="margin-top:10px; display:flex; gap:6px; flex-wrap:wrap;">
        <button type="button" class="alias-filter-chip ${poolTypeFilter === 'all' ? 'active' : ''}" data-filter-pool-type="all" style="font-size:10px; padding:4px 9px;">
          All Types (${allRows.length})
        </button>
        ${['vercel', 'cloudflare', 'deno', 'http'].map((t) => typeCounts[t] > 0 ? `
          <button type="button" class="alias-filter-chip ${poolTypeFilter === t ? 'active' : ''}" data-filter-pool-type="${t}" style="font-size:10px; padding:4px 9px;">
            ${t.toUpperCase()} (${typeCounts[t]})
          </button>
        ` : '').join('')}
      </div>

      <!-- Live Health Check Progress Bar (Hidden when idle) -->
      <div id="pool-health-progress-wrap" style="display:none; margin-top:12px; padding:10px 12px; background:#05080c; border:1px solid rgba(200,255,99,0.25); border-radius:6px;">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px; font-family:var(--mono); font-size:10.5px;">
          <span id="pool-health-progress-label" style="color:var(--lime); font-weight:600;">Checking Proxy Pool Health...</span>
          <span id="pool-health-progress-stats" style="color:var(--muted);">0 / ${allRows.length}</span>
        </div>
        <div style="width:100%; height:4px; background:rgba(255,255,255,0.06); border-radius:2px; overflow:hidden;">
          <div id="pool-health-progress-fill" style="width:0%; height:100%; background:var(--lime); transition:width 0.15s ease;"></div>
        </div>
      </div>
    </div>

    ${table}
  `;
}

// ─────────────────────────────────────────────────────────────
// 6. ROUTER & VIEW RENDERER
// ─────────────────────────────────────────────────────────────
async function renderView(name) {
  if (name === 'overview') return;
  if (name.startsWith('provider/')) {
    const provId = name.split('/')[1];
    await renderProviderDetail(provId);
    return;
  }

  content.innerHTML = '<div class="card generic-empty"><span class="loading-line"></span><p>Reading backend state...</p></div>';
  try {
    let payload;
    payload = await ({
        providers: async () => {
          const [connsRes, nodesRes] = await Promise.all([
            request('/api/providers').catch(() => ({ connections: [] })),
            request('/api/provider-nodes').catch(() => ({ nodes: [] }))
          ]);
          return {
            connections: connsRes.connections || [],
            nodes: nodesRes.nodes || []
          };
        },
        orchestrator: () => request('/api/combos'),
        keys: () => request('/api/keys'),
        usage: () => request('/api/usage/stats?period=all&days=all'),
        logs: () => request('/api/usage/stats?period=all&days=all').catch(() => request('/translator/console-logs')).catch(() => ({ recentRequests: [] })),
        pools: () => request('/api/proxy-pools'),
        aliases: () => request('/api/model-aliases'),
        settings: () => request('/api/settings')
      }[name] || (() => Promise.resolve(null)))();
    content.innerHTML = name === 'providers' ? renderProviders(payload) : name === 'orchestrator' ? renderCombos(payload) : name === 'keys' ? renderKeys(payload) : name === 'usage' ? renderUsage(payload) : name === 'logs' ? renderLogs(payload) : name === 'pools' ? renderPools(payload) : name === 'aliases' ? renderAliases(payload) : renderSettings(payload);
    
    if (name === 'settings') bindSettings();
    if (name === 'pools') bindDeployButtons();
    if (name === 'usage') bindUsageFilters();
    bindCreateForm(name);
    bindDeleteButtons(name);
    if (name === 'providers') {
      bindProviderGroupActions();
    }
    if (name === 'orchestrator') {
      bindComboEditors();
    }
    if (name === 'keys') {
      bindKeyPolicyEditors();
      bindCopyKeyButtons();
    }
    if (name === 'logs') bindLogStream();
    if (name === 'aliases') bindAliasDeckActions();
  } catch (error) {
    const isAuthErr = error.status === 401 ||
      (error.message && (error.message.includes('401') || error.message.toLowerCase().includes('unauthorized') || error.message.toLowerCase().includes('token') || error.message.toLowerCase().includes('expired'))) ||
      !hasDashboardAccess();
    if (isAuthErr) {
      renderFullLoginGate();
    } else {
      const displayMsg = error.message || (typeof error === 'object' ? JSON.stringify(error) : String(error));
      content.innerHTML = emptySurface(`Backend error: ${escapeHtml(displayMsg)}`);
    }
  }
}

function bindCopyKeyButtons() {
  document.querySelectorAll('[data-copy-key-id]').forEach((btn) => {
    btn.onclick = async () => {
      try {
        const payload = await request(`/api/keys/${encodeURIComponent(btn.dataset.copyKeyId)}/reveal`);
        await copyText(payload.key);
        const prev = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = prev; }, 1200);
      } catch (error) {
        showToast(error.message || 'Copy failed', 'error');
      }
    };
  });
}

function bindUsageFilters(activeDays = 'all', activeProv = '', activeModel = '') {
  const form = document.querySelector('#usage-filters');
  if (!form) return;
  if (activeDays) {
    const select = form.querySelector('select[name="days"]');
    if (select) select.value = activeDays;
  }
  if (activeProv) {
    const provIn = form.querySelector('input[name="provider"]');
    if (provIn) provIn.value = activeProv;
  }
  if (activeModel) {
    const modIn = form.querySelector('input[name="model"]');
    if (modIn) modIn.value = activeModel;
  }
  const prevRecentBtn = document.querySelector('#btn-usage-recent-prev');
  if (prevRecentBtn) {
    prevRecentBtn.onclick = () => {
      if (usageRecentPage > 1) {
        usageRecentPage--;
        content.innerHTML = renderUsage();
        bindUsageFilters(activeDays, activeProv, activeModel);
      }
    };
  }

  const nextRecentBtn = document.querySelector('#btn-usage-recent-next');
  if (nextRecentBtn) {
    nextRecentBtn.onclick = () => {
      usageRecentPage++;
      content.innerHTML = renderUsage();
      bindUsageFilters(activeDays, activeProv, activeModel);
    };
  }

  form.onsubmit = async (event) => {
    event.preventDefault();
    const values = Object.fromEntries(new FormData(form).entries());
    const selectedDays = values.days || 'all';
    const periodVal = selectedDays === 'all' ? 'all' : selectedDays === '90' ? '60d' : selectedDays === '30' ? '30d' : '7d';
    const params = new URLSearchParams();
    params.set('period', periodVal);
    params.set('days', selectedDays);
    if (values.provider) params.set('provider', values.provider.trim());
    if (values.model) params.set('model', values.model.trim());
    try {
      usageRecentPage = 1;
      const payload = await request(`/api/usage/stats?${params.toString()}`);
      content.innerHTML = renderUsage(payload);
      bindUsageFilters(values.days, values.provider, values.model);
    } catch (error) {
      content.innerHTML = emptySurface(`Usage unavailable: ${error.message}`);
    }
  };

  // Audit Files Vault Loader
  const loadAuditFiles = async () => {
    const slot = document.querySelector('#audit-files-table-slot');
    if (!slot) return;
    try {
      const res = await fetch(`${apiBase}/api/audit-logs/files`, { headers: getHeaders() });
      const data = await res.json().catch(() => ({ files: [] }));
      const files = data.files || [];

      if (files.length === 0) {
        slot.innerHTML = `
          <div style="text-align:center; padding:24px; color:var(--muted); font-size:11.5px; border:1px dashed var(--line); border-radius:6px;">
            No audit log files recorded yet. Run requests to start recording unredacted transactions.
          </div>
        `;
        return;
      }

      slot.innerHTML = `
        <div class="data-table-container" style="border:1px solid var(--line); border-radius:6px; overflow:hidden;">
          <table class="data-table">
            <thead>
              <tr>
                <th style="width:90px;">Status</th>
                <th>Log File Name</th>
                <th style="width:120px;">File Size</th>
                <th style="width:180px;">Last Modified</th>
                <th class="table-cell-actions" style="width:160px; text-align:right;">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${files.map((f) => `
                <tr>
                  <td>
                    <span class="table-badge ${f.isActive ? 'active' : 'inactive'}" style="font-size:7.5px;">
                      ${f.isActive ? 'RECORDING' : 'ARCHIVED'}
                    </span>
                  </td>
                  <td>
                    <div style="display:flex; align-items:center; gap:6px;">
                      <span class="material-symbols-outlined" style="font-size:16px; color:${f.isActive ? 'var(--lime)' : 'var(--muted)'};">description</span>
                      <code class="model-id-code" style="font-size:11px; font-weight:600; color:var(--text-bright);">${escapeHtml(f.filename)}</code>
                    </div>
                  </td>
                  <td><span class="table-cell-mono" style="font-size:11px; color:#38bdf8;">${escapeHtml(f.sizeMB || '0 MB')}</span></td>
                  <td><span style="font-size:10.5px; font-family:var(--mono); color:var(--muted);">${escapeHtml(f.modTime ? new Date(f.modTime).toLocaleString() : '--')}</span></td>
                  <td class="table-cell-actions" style="text-align:right;">
                    <a href="${apiBase}/api/audit-logs/files/${encodeURIComponent(f.filename)}" target="_blank" download="${escapeHtml(f.filename)}" class="secondary-button" style="text-decoration:none; font-size:9.5px; padding:3px 8px; display:inline-flex; align-items:center; gap:4px;">
                      <span class="material-symbols-outlined" style="font-size:12px;">download</span> Download
                    </a>
                  </td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        </div>
      `;
    } catch (err) {
      slot.innerHTML = `<p style="color:var(--danger); font-size:11px; padding:12px;">Failed to load audit file list: ${escapeHtml(err.message)}</p>`;
    }
  };

  loadAuditFiles();
  const refreshAuditBtn = document.querySelector('#btn-refresh-audit-files');
  if (refreshAuditBtn) {
    refreshAuditBtn.onclick = () => {
      loadAuditFiles();
      showToast('Audit log file list refreshed', 'info');
    };
  }
}

function bindDeleteButtons(name) {
  content.querySelectorAll('[data-delete]').forEach((button) => {
    button.onclick = async () => {
      const confirmed = await showConfirmModal({
        title: `Delete ${name.toUpperCase()}`,
        kicker: 'DELETE RECORD',
        message: `Are you sure you want to delete this ${name} item from SQLite? This action cannot be undone.`,
        confirmText: 'Delete',
        danger: true
      });
      if (!confirmed) return;
      const endpoint = name === 'providers' ? '/api/providers/' : name === 'orchestrator' ? '/api/combos/' : name === 'keys' ? '/api/keys/' : name === 'aliases' ? '/api/model-aliases/' : '/api/proxy-pools/';
      try {
        const response = await fetch(`${apiBase}${endpoint}${encodeURIComponent(button.dataset.id)}`, { method: 'DELETE', headers });
        if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
        showToast(`${name.toUpperCase()} item deleted`, 'info');
        await renderView(name);
      } catch (error) {
        showToast(`Delete failed: ${error.message}`, 'error');
      }
    };
  });
}

function parseRestrictionsObject(raw) {
  if (!raw) return { allowedModels: [], allowedPrefixes: [], allowedProviders: [], blockedModels: [], rateLimit: { requestsPerMinute: 0, tokensPerDay: 0 } };
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw;
    return {
      allowedModels: Array.isArray(obj.allowedModels) ? obj.allowedModels : [],
      allowedPrefixes: Array.isArray(obj.allowedPrefixes) ? obj.allowedPrefixes : [],
      allowedProviders: Array.isArray(obj.allowedProviders) ? obj.allowedProviders : [],
      blockedModels: Array.isArray(obj.blockedModels) ? obj.blockedModels : [],
      rateLimit: obj.rateLimit || { requestsPerMinute: 0, tokensPerDay: 0 }
    };
  } catch {
    return { allowedModels: [], allowedPrefixes: [], allowedProviders: [], blockedModels: [], rateLimit: { requestsPerMinute: 0, tokensPerDay: 0 } };
  }
}
function getActiveProviderModels(allConnections = [], allBackendModels = [], providerNodes = [], customModels = []) {
  const activeConns = (allConnections || []).filter(isItemActive);
  const activeProviderMap = new Map();
  const allActiveModels = [];
  const suggestedPrefixes = new Set();
  const nodeMap = new Map((providerNodes || []).map((node) => [String(node.id || '').toLowerCase(), node]));

  const ensureProviderGroup = (provId) => {
    if (!provId) return null;
    const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === provId || (p.alias && p.alias === provId));
    const canonicalId = cat?.id || provId;
    if (activeProviderMap.has(canonicalId)) return activeProviderMap.get(canonicalId);
    const node = nodeMap.get(canonicalId) || nodeMap.get(provId);
    // Public/no-auth providers have no providerConnections row, but they are
    // still active routing targets and must appear in policy restrictions.
    if (!cat || (cat.category !== 'free' && cat.authType !== 'noauth')) return null;
    const defaultPrefix = cat?.alias || canonicalId;
    const entry = {
      provId: canonicalId,
      providerName: node?.name || cat?.name || canonicalId.toUpperCase(),
      conns: [],
      modelSet: new Set(cat?.defaultModels || []),
      routePrefix: node?.prefix || defaultPrefix
    };
    activeProviderMap.set(canonicalId, entry);
    return entry;
  };

  // 1. Group active connections by unique Provider Type
  activeConns.forEach((conn) => {
    const rawProv = (conn.provider || '').toLowerCase();
    if (!rawProv) return;
    const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === rawProv || (p.alias && p.alias === rawProv));
    const provId = cat?.id || rawProv;

    if (!activeProviderMap.has(provId)) {
      const node = nodeMap.get(provId) || nodeMap.get(rawProv);
      const defaultPrefix = cat?.alias || provId;
      activeProviderMap.set(provId, {
        provId,
        providerName: node?.name || cat?.name || provId.toUpperCase(),
        conns: [],
        modelSet: new Set(cat?.defaultModels || []),
        routePrefix: node?.prefix || defaultPrefix
      });
    }

    const entry = activeProviderMap.get(provId);
    entry.conns.push(conn);

    // Add custom models from connection data
    try {
      const d = typeof conn.data === 'string' ? JSON.parse(conn.data) : (conn.data || {});
      const connectionPrefix = String(d.prefix || d.providerSpecificData?.prefix || '').trim().toLowerCase();
      if (connectionPrefix) entry.routePrefix = connectionPrefix;
      if (d.defaultModel) entry.modelSet.add(d.defaultModel);
      if (Array.isArray(d.customModels)) d.customModels.forEach((cm) => entry.modelSet.add(cm));
    } catch {}
  });

  // Include models manually registered in the provider detail view. These are
  // not guaranteed to be returned by the live /models endpoint.
  (customModels || []).forEach((model) => {
    const provider = String(model.provider || model.providerId || '').toLowerCase();
    const alias = String(model.providerAlias || '').toLowerCase();
    const entry = activeProviderMap.get(provider) || Array.from(activeProviderMap.values()).find((candidate) => {
      const node = nodeMap.get(candidate.provId);
      const candidateAliases = [
        candidate.provId,
        candidate.routePrefix,
        node?.prefix,
        node?.providerAlias,
        node?.name
      ].map((value) => String(value || '').toLowerCase()).filter(Boolean);
      return candidateAliases.includes(provider) || candidateAliases.includes(alias);
    });
    if (!entry || !model.id) return;
    entry.modelSet.add(String(model.id).trim());
  });

  // 2. Add backend /models matching active providers
  (allBackendModels || []).forEach((m) => {
    const mid = typeof m === 'string' ? m : m.id;
    const owner = typeof m === 'object' ? (m.owned_by || '').toLowerCase() : '';
    if (owner) {
      const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === owner || (p.alias && p.alias === owner));
      const canonicalOwner = cat?.id || owner;
      const entry = activeProviderMap.get(canonicalOwner) || ensureProviderGroup(canonicalOwner);
      if (entry) entry.modelSet.add(mid);
    }
  });

  // 3. Build distinct provider groups and suggested prefixes
  const groups = [];
  activeProviderMap.forEach((entry, provId) => {
    const rawModels = Array.from(entry.modelSet)
      .map((model) => {
        const normalized = String(model || '').trim();
        if (!normalized) return '';
        const prefix = entry.routePrefix || provId;
        if (normalized.includes('/')) {
          const parts = normalized.split('/');
          const curPrefix = parts[0];
          const curModel = parts[1];
          const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === provId || (p.alias && p.alias === provId));
          if (curPrefix.toLowerCase() === prefix.toLowerCase() ||
              (cat && (curPrefix.toLowerCase() === cat.id.toLowerCase() || (cat.alias && curPrefix.toLowerCase() === cat.alias.toLowerCase())))) {
            return `${prefix}/${curModel}`;
          }
          return normalized;
        }
        return `${prefix}/${normalized}`;
      })
      .filter(Boolean);
    rawModels.forEach((m) => allActiveModels.push(m));

    // Suggested wildcard prefixes for this active provider
    suggestedPrefixes.add(`${provId}/*`);
    if (provId.includes('openai')) {
      suggestedPrefixes.add('gpt-*');
      suggestedPrefixes.add('o1-*');
    }
    if (provId.includes('anthropic') || provId.includes('claude')) {
      suggestedPrefixes.add('claude-*');
    }
    if (provId.includes('gemini') || provId.includes('google')) {
      suggestedPrefixes.add('gemini-*');
    }
    if (provId.includes('deepseek')) {
      suggestedPrefixes.add('deepseek-*');
    }
    if (provId.includes('groq')) {
      suggestedPrefixes.add('llama-*');
    }
    if (provId.includes('mistral')) {
      suggestedPrefixes.add('mistral-*');
    }
    if (provId.includes('xai') || provId.includes('grok')) {
      suggestedPrefixes.add('grok-*');
    }

    groups.push({
      provider: provId,
      providerName: entry.providerName,
      accountCount: entry.conns.length,
      models: Array.from(new Set(rawModels))
    });
  });

  return {
    activeConnections: activeConns,
    groups,
    allActiveModels: Array.from(new Set(allActiveModels)),
    suggestedPrefixes: Array.from(suggestedPrefixes)
  };
}
function keyPolicyForm(item, isNew = false, availableProviders = [], availableModels = [], providerNodes = [], customModels = []) {
  const current = parseRestrictionsObject(item.restrictions);
  const { activeConnections, groups, allActiveModels, suggestedPrefixes } = getActiveProviderModels(availableProviders, availableModels, providerNodes, customModels);
  const uniquePrefixes = Array.from(new Set([...suggestedPrefixes, ...current.allowedPrefixes]));
  const isModelSelected = (model) => current.allowedModels.some((allowed) => {
    const allowedName = String(allowed || '').split('/').pop();
    const modelName = String(model || '').split('/').pop();
    return String(allowed).toLowerCase() === String(model).toLowerCase() || allowedName.toLowerCase() === modelName.toLowerCase();
  });

  return `
    <form class="inline-form policy-builder-form" id="key-policy-form" data-key-id="${escapeHtml(item.id || '')}" data-provider-groups="${escapeHtml(JSON.stringify(groups.map((g) => ({ provider: g.provider, providerName: g.providerName }))))}">
      <div class="form-head">
        <div>
          <span class="kicker">RESTRICTIONS</span>
          <h2>${isNew ? 'New API Key Policy' : `Policy: ${escapeHtml(item.name || item.id)}`}</h2>
        </div>
        <div class="mode-tabs">
          <button type="button" class="mode-tab active" data-mode="visual">Visual Builder</button>
          <button type="button" class="mode-tab" data-mode="raw">Raw JSON</button>
        </div>
      </div>

      <div class="form-grid-2">
        <label>
          API Key Name
          <input name="name" id="policy-key-name" value="${escapeHtml(item.name || '')}" placeholder="e.g. Production Agent Key" required />
        </label>
        <label>
          Active State
          <select name="isActive" id="policy-key-active">
            <option value="1" ${item.isActive !== 0 ? 'selected' : ''}>Active (Enabled)</option>
            <option value="0" ${item.isActive === 0 ? 'selected' : ''}>Disabled (Revoked)</option>
          </select>
        </label>
      </div>

      <!-- VISUAL BUILDER SURFACE -->
      <div id="visual-builder-surface">
        <div id="policy-conflict-warning" class="hidden"></div>
        <div class="builder-section">
          <div class="section-title">
            <strong>1. Allowed Models Whitelist</strong>
            <small>Leave empty to allow all models. Select specific models to restrict access.</small>
          </div>
          <div class="chip-container" id="selected-models-container">
            ${current.allowedModels.length === 0 ? '<span class="empty-chip-note">All models allowed (no whitelist)</span>' : current.allowedModels.map((m) => `<span class="chip active-chip" data-model="${escapeHtml(m)}">${escapeHtml(m)} <i class="remove-chip">&times;</i></span>`).join('')}
          </div>

          ${groups.length > 0 ? `
            <div class="quick-add-bar" style="display:grid; gap:6px;">
              <span class="quick-add-label">ACTIVE MODELS:</span>
              ${groups.map((grp) => `
                <div class="active-prov-model-group" style="background:#080b10; border:1px solid var(--line-subtle); border-radius:5px; padding:6px 8px;">
                  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:4px;">
                    <strong style="font-size:10.5px; color:var(--text-bright);">${escapeHtml(grp.providerName)}</strong>
                    <span style="font-size:8.5px; font-family:var(--mono); color:var(--muted);">${grp.accountCount > 0 ? `${grp.accountCount} active` : 'public / no-auth'}</span>
                  </div>
                  <div class="quick-chips" style="display:flex; flex-wrap:wrap; gap:3px;">
                    ${grp.models.map((m) => `
                      <button type="button" class="preset-chip ${isModelSelected(m) ? 'picked' : ''}" data-add-model="${escapeHtml(m)}">
                        + ${escapeHtml(m)}
                      </button>
                    `).join('')}
                  </div>
                </div>
              `).join('')}
            </div>
          ` : `
            <p style="color:var(--muted); font-size:11px; margin:4px 0 0;">No active providers connected.</p>
          `}

          <div class="custom-input-row" style="margin-top:6px;">
            <input type="text" id="custom-model-input" placeholder="Type custom model ID (e.g. gpt-4o, claude-3-5-sonnet)..." />
            <button type="button" class="secondary-button" id="btn-add-custom-model">+ Add</button>
          </div>
        </div>

        <div class="builder-section">
          <div class="section-title">
            <strong>2. Wildcard Prefix Rules</strong>
            <small>Match entire model families (e.g. claude-*, gpt-*).</small>
          </div>
          <div class="chip-container" id="selected-prefixes-container">
            ${current.allowedPrefixes.length === 0 ? '<span class="empty-chip-note">No prefix rules applied</span>' : current.allowedPrefixes.map((p) => `<span class="chip purple-chip" data-prefix="${escapeHtml(p)}">${escapeHtml(p)} <i class="remove-chip">&times;</i></span>`).join('')}
          </div>
          ${uniquePrefixes.length > 0 ? `
            <div class="quick-add-bar">
              <span class="quick-add-label">SUGGESTED:</span>
              <div class="quick-chips">
                ${uniquePrefixes.map((p) => `<button type="button" class="preset-chip ${current.allowedPrefixes.includes(p) ? 'picked' : ''}" data-add-prefix="${escapeHtml(p)}">${escapeHtml(p)}</button>`).join('')}
              </div>
            </div>
          ` : ''}
          <div class="custom-input-row">
            <input type="text" id="custom-prefix-input" placeholder="Custom prefix pattern (e.g. mistral-*)..." />
            <button type="button" class="secondary-button" id="btn-add-custom-prefix">+ Add</button>
          </div>
        </div>

        <!-- SECTION 3: PROVIDER LOCKING -->
        <div class="builder-section">
          <div class="section-title">
            <div style="display:flex; justify-content:space-between; align-items:center;">
              <strong>3. Provider Locking (Optional)</strong>
              <span id="prov-lock-status-badge" class="table-badge ${current.allowedProviders.length === 0 ? 'active' : 'purple'}" style="font-size:8px;">
                ${current.allowedProviders.length === 0 ? 'ALL ALLOWED' : `LOCKED (${current.allowedProviders.length})`}
              </span>
            </div>
            <small>Leave empty to allow all providers. Check specific providers to restrict routing.</small>
          </div>

          ${groups.length === 0 ? `
            <p style="color:var(--muted); font-size:11px; margin:4px 0 0;">No active providers connected.</p>
          ` : `
            <div class="prov-locking-controls" style="display:flex; justify-content:space-between; align-items:center; gap:8px; margin:4px 0 6px;">
              <span style="font-size:9px; font-family:var(--mono); color:var(--muted);">PROVIDERS (${groups.length}):</span>
              <button type="button" class="secondary-button" id="btn-unlock-all-prov" style="font-size:9px; padding:2px 6px;">Allow All</button>
            </div>

            <div class="provider-checkbox-grid" style="display:grid; grid-template-columns:repeat(auto-fill, minmax(200px, 1fr)); gap:5px;">
              ${groups.map((grp) => {
                const isChecked = current.allowedProviders.includes(grp.provider);
                return `
                  <label class="provider-check-card ${isChecked ? 'checked' : ''}" style="padding:6px 8px;">
                    <input type="checkbox" name="allowedProviders" value="${escapeHtml(grp.provider)}" ${isChecked ? 'checked' : ''} />
                    ${renderProviderIcon(grp.provider)}
                    <div style="min-width:0; flex:1;">
                      <strong style="display:block; font-size:11px; color:var(--text-bright); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${escapeHtml(grp.providerName)}</strong>
                      <small style="display:block; font-size:8.5px; font-family:var(--mono); color:var(--muted);">${grp.accountCount} active</small>
                    </div>
                  </label>
                `;
              }).join('')}
            </div>
          `}
        </div>

        <div class="builder-section">
          <div class="section-title">
            <strong>4. Rate Limits & Quotas</strong>
            <small>Throttle usage per key (0 = Unlimited).</small>
          </div>
          <div class="form-grid-2">
            <label>
              RPM (Requests/Min)
              <input type="number" id="limit-rpm" min="0" value="${escapeHtml(current.rateLimit?.requestsPerMinute ?? 0)}" placeholder="0" />
            </label>
            <label>
              TPD (Tokens/Day)
              <input type="number" id="limit-tpd" min="0" value="${escapeHtml(current.rateLimit?.tokensPerDay ?? 0)}" placeholder="0" />
            </label>
          </div>
        </div>
      </div>

      <!-- RAW JSON SURFACE (Hidden by default) -->
      <div id="raw-json-surface" class="hidden">
        <label>
          Restrictions JSON Blob
          <textarea id="policy-raw-json" rows="10" style="font-family: var(--mono); font-size: 11px;">${escapeHtml(JSON.stringify(current, null, 2))}</textarea>
        </label>
      </div>

      <div class="form-actions">
        <button class="solid-button" type="submit">Save Policy</button>
        <button class="cancel-button" type="button" id="btn-cancel-policy">Cancel</button>
      </div>
      <p class="form-error" role="alert"></p>
    </form>
  `;
}

function bindKeyPolicyEditors() {
  document.querySelectorAll('[data-edit-key]').forEach((button) => {
    button.onclick = async () => {
      try {
        const [keysPayload, provPayload, modelPayload, nodesPayload, customPayload] = await Promise.all([
          request('/api/keys'),
          request('/api/providers').catch(() => ({ connections: [] })),
          request('/models').catch(() => ({ data: [] })),
          request('/api/provider-nodes').catch(() => ({ nodes: [] })),
          request('/api/custom-models').catch(() => ({ customModels: [] }))
        ]);
        const item = (keysPayload.keys || []).find((k) => k.id === button.dataset.editKey);
        if (!item) throw new Error('API key not found');
        const connections = provPayload.connections || [];
        const models = modelPayload.data || [];

        const existingForm = document.querySelector('#key-policy-form');
        if (existingForm) existingForm.remove();

        content.insertAdjacentHTML('afterbegin', keyPolicyForm(item, false, connections, models, nodesPayload.nodes || [], customPayload.customModels || []));
        setupPolicyBuilderInteractions(item, false);
      } catch (err) {
        alert(`Failed to open policy editor: ${err.message}`);
      }
    };
  });
}


function setupPolicyBuilderInteractions(item, isNew = false) {
  const form = document.querySelector('#key-policy-form');
  if (!form) return;

  const state = parseRestrictionsObject(item.restrictions);
  let groups = [];
  try {
    groups = JSON.parse(form.dataset.providerGroups || '[]');
  } catch {}

  function checkPolicyDeadlocks() {
    const warningBox = form.querySelector('#policy-conflict-warning');
    if (!warningBox) return;

    const checkedProvs = Array.from(form.querySelectorAll('input[name="allowedProviders"]:checked')).map((cb) => cb.value.toLowerCase());
    if (checkedProvs.length === 0 || state.allowedModels.length === 0) {
      warningBox.classList.add('hidden');
      warningBox.innerHTML = '';
      return;
    }

    const conflicts = [];
    state.allowedModels.forEach((model) => {
      const parts = String(model || '').split('/');
      if (parts.length === 2) {
        const prefix = parts[0].toLowerCase();
        // find matching provider for this prefix
        const matchingGroup = (groups || []).find((g) => {
          const provId = String(g.provider || '').toLowerCase();
          const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === provId || (p.alias && p.alias === provId));
          return provId === prefix || (cat && (cat.id.toLowerCase() === prefix || (cat.alias && cat.alias.toLowerCase() === prefix)));
        });
        if (matchingGroup) {
          const provId = String(matchingGroup.provider || '').toLowerCase();
          const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === provId);
          const alias = cat?.alias ? String(cat.alias).toLowerCase() : null;
          const isAllowed = checkedProvs.includes(provId) || (alias && checkedProvs.includes(alias));
          if (!isAllowed) {
            conflicts.push({ model, providerName: matchingGroup.providerName, provId: matchingGroup.provider });
          }
        }
      }
    });

    if (conflicts.length > 0) {
      warningBox.classList.remove('hidden');
      warningBox.innerHTML = `
        <div style="background: rgba(239, 68, 68, 0.12); border: 1px solid #ef4444; border-radius: 6px; padding: 10px 12px; margin-bottom: 12px; display: flex; flex-direction: column; gap: 6px;">
          <div style="display: flex; align-items: center; gap: 6px; color: #f87171; font-weight: 600; font-size: 11.5px;">
            <span class="material-symbols-outlined" style="font-size: 16px;">warning</span>
            <span>Policy Conflict / Deadlock Detected!</span>
          </div>
          <p style="font-size: 11px; color: #fca5a5; margin: 0; line-height: 1.4;">
            Allowed Providers is locked to <code>${escapeHtml(checkedProvs.join(', '))}</code>, but Allowed Models contains models from other providers:
            <br>
            ${conflicts.map((c) => `• <code>${escapeHtml(c.model)}</code> (Belongs to: <strong>${escapeHtml(c.providerName)}</strong>)`).join('<br>')}
          </p>
          <small style="font-size: 10px; color: #cbd5e1;">With this configuration, clients with this key will receive <strong>0 models</strong> or <strong>HTTP 403 Forbidden</strong>.</small>
          <div style="display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px;">
            <button type="button" id="btn-fix-conflict-add-providers" class="secondary-button" style="font-size: 10px; padding: 4px 8px; color: #38bdf8; border-color: #38bdf8;">
              + Auto-Add Missing Providers (${escapeHtml(Array.from(new Set(conflicts.map((c) => c.provId))).join(', '))})
            </button>
            <button type="button" id="btn-fix-conflict-unlock" class="secondary-button" style="font-size: 10px; padding: 4px 8px;">
              Unlock All Providers (Allow All)
            </button>
          </div>
        </div>
      `;

      const btnAddMissing = warningBox.querySelector('#btn-fix-conflict-add-providers');
      if (btnAddMissing) {
        btnAddMissing.onclick = () => {
          conflicts.forEach((c) => {
            const cb = form.querySelector(`input[name="allowedProviders"][value="${c.provId}"]`);
            if (cb) {
              cb.checked = true;
              cb.closest('.provider-check-card')?.classList.add('checked');
            }
          });
          syncToRawJson();
        };
      }
      const btnUnlock = warningBox.querySelector('#btn-fix-conflict-unlock');
      if (btnUnlock) {
        btnUnlock.onclick = () => {
          form.querySelectorAll('input[name="allowedProviders"]').forEach((cb) => {
            cb.checked = false;
            cb.closest('.provider-check-card')?.classList.remove('checked');
          });
          syncToRawJson();
        };
      }
    } else {
      warningBox.classList.add('hidden');
      warningBox.innerHTML = '';
    }
  }

  function updateProvLockBadge() {
    const checked = form.querySelectorAll('input[name="allowedProviders"]:checked');
    const badge = form.querySelector('#prov-lock-status-badge');
    if (badge) {
      if (checked.length === 0) {
        badge.className = 'table-badge active';
        badge.textContent = 'DEFAULT: ALL CONNECTIONS ALLOWED';
      } else {
        badge.className = 'table-badge purple';
        badge.textContent = `LOCKED TO ${checked.length} TARGETS`;
      }
    }
  }

  function syncToRawJson() {
    state.rateLimit.requestsPerMinute = Number(form.querySelector('#limit-rpm').value) || 0;
    state.rateLimit.tokensPerDay = Number(form.querySelector('#limit-tpd').value) || 0;
    const checkedProv = Array.from(form.querySelectorAll('input[name="allowedProviders"]:checked')).map((cb) => cb.value);
    state.allowedProviders = Array.from(new Set(checkedProv));
    form.querySelector('#policy-raw-json').value = JSON.stringify(state, null, 2);
    updateProvLockBadge();
    checkPolicyDeadlocks();
  }

  function renderModelChips() {
    const container = form.querySelector('#selected-models-container');
    if (state.allowedModels.length === 0) {
      container.innerHTML = '<span class="empty-chip-note">All models allowed (no whitelist restriction)</span>';
    } else {
      container.innerHTML = state.allowedModels.map((m) => `<span class="chip active-chip" data-model="${escapeHtml(m)}">${escapeHtml(m)} <i class="remove-chip">&times;</i></span>`).join('');
    }
    form.querySelectorAll('[data-add-model]').forEach((btn) => {
      btn.classList.toggle('picked', state.allowedModels.includes(btn.dataset.addModel));
    });
    syncToRawJson();
  }

  function renderPrefixChips() {
    const container = form.querySelector('#selected-prefixes-container');
    if (state.allowedPrefixes.length === 0) {
      container.innerHTML = '<span class="empty-chip-note">No prefix rules applied</span>';
    } else {
      container.innerHTML = state.allowedPrefixes.map((p) => `<span class="chip purple-chip" data-prefix="${escapeHtml(p)}">${escapeHtml(p)} <i class="remove-chip">&times;</i></span>`).join('');
    }
    form.querySelectorAll('[data-add-prefix]').forEach((btn) => {
      btn.classList.toggle('picked', state.allowedPrefixes.includes(btn.dataset.addPrefix));
    });
    syncToRawJson();
  }

  // Model chips removal & addition
  form.querySelector('#selected-models-container').onclick = (e) => {
    const chip = e.target.closest('[data-model]');
    if (chip && e.target.classList.contains('remove-chip')) {
      const model = chip.dataset.model;
      state.allowedModels = state.allowedModels.filter((m) => m !== model);
      renderModelChips();
    }
  };

  form.querySelectorAll('[data-add-model]').forEach((btn) => {
    btn.onclick = () => {
      const model = btn.dataset.addModel;
      if (state.allowedModels.includes(model)) {
        state.allowedModels = state.allowedModels.filter((m) => m !== model);
      } else {
        state.allowedModels.push(model);
      }
      renderModelChips();
    };
  });

  const btnAddCustomModel = form.querySelector('#btn-add-custom-model');
  const inputCustomModel = form.querySelector('#custom-model-input');
  if (btnAddCustomModel && inputCustomModel) {
    const addCustom = () => {
      const val = inputCustomModel.value.trim();
      if (val && !state.allowedModels.includes(val)) {
        state.allowedModels.push(val);
        inputCustomModel.value = '';
        renderModelChips();
      }
    };
    btnAddCustomModel.onclick = addCustom;
    inputCustomModel.onkeydown = (e) => { if (e.key === 'Enter') { e.preventDefault(); addCustom(); } };
  }

  // Prefix chips removal & addition
  form.querySelector('#selected-prefixes-container').onclick = (e) => {
    const chip = e.target.closest('[data-prefix]');
    if (chip && e.target.classList.contains('remove-chip')) {
      const prefix = chip.dataset.prefix;
      state.allowedPrefixes = state.allowedPrefixes.filter((p) => p !== prefix);
      renderPrefixChips();
    }
  };

  form.querySelectorAll('[data-add-prefix]').forEach((btn) => {
    btn.onclick = () => {
      const prefix = btn.dataset.addPrefix;
      if (state.allowedPrefixes.includes(prefix)) {
        state.allowedPrefixes = state.allowedPrefixes.filter((p) => p !== prefix);
      } else {
        state.allowedPrefixes.push(prefix);
      }
      renderPrefixChips();
    };
  });

  const btnAddCustomPrefix = form.querySelector('#btn-add-custom-prefix');
  const inputCustomPrefix = form.querySelector('#custom-prefix-input');
  if (btnAddCustomPrefix && inputCustomPrefix) {
    const addPrefix = () => {
      const val = inputCustomPrefix.value.trim();
      if (val && !state.allowedPrefixes.includes(val)) {
        state.allowedPrefixes.push(val);
        inputCustomPrefix.value = '';
        renderPrefixChips();
      }
    };
    btnAddCustomPrefix.onclick = addPrefix;
    inputCustomPrefix.onkeydown = (e) => { if (e.key === 'Enter') { e.preventDefault(); addPrefix(); } };
  }

  // Provider Locking Controls & Checkboxes
  const unlockAllBtn = form.querySelector('#btn-unlock-all-prov');
  if (unlockAllBtn) {
    unlockAllBtn.onclick = () => {
      form.querySelectorAll('input[name="allowedProviders"]').forEach((cb) => {
        cb.checked = false;
        cb.closest('.provider-check-card')?.classList.remove('checked');
      });
      syncToRawJson();
    };
  }

  form.querySelectorAll('.provider-check-card input').forEach((cb) => {
    cb.onchange = () => {
      cb.closest('.provider-check-card')?.classList.toggle('checked', cb.checked);
      syncToRawJson();
    };
  });

  updateProvLockBadge();
  checkPolicyDeadlocks();

  // Limits
  form.querySelector('#limit-rpm').oninput = syncToRawJson;
  form.querySelector('#limit-tpd').oninput = syncToRawJson;

  // Mode Switcher (Visual vs Raw JSON)
  form.querySelectorAll('.mode-tab').forEach((tab) => {
    tab.onclick = () => {
      form.querySelectorAll('.mode-tab').forEach((t) => t.classList.remove('active'));
      tab.classList.add('active');
      const isVisual = tab.dataset.mode === 'visual';
      form.querySelector('#visual-builder-surface').classList.toggle('hidden', !isVisual);
      form.querySelector('#raw-json-surface').classList.toggle('hidden', isVisual);
      if (isVisual) {
        try {
          const raw = JSON.parse(form.querySelector('#policy-raw-json').value);
          Object.assign(state, parseRestrictionsObject(raw));
          renderModelChips();
          renderPrefixChips();
          form.querySelector('#limit-rpm').value = state.rateLimit?.requestsPerMinute ?? 0;
          form.querySelector('#limit-tpd').value = state.rateLimit?.tokensPerDay ?? 0;
          form.querySelectorAll('input[name="allowedProviders"]').forEach((cb) => {
            cb.checked = state.allowedProviders.includes(cb.value);
            cb.closest('.provider-check-card')?.classList.toggle('checked', cb.checked);
          });
          updateProvLockBadge();
        } catch {}
      } else {
        syncToRawJson();
      }
    };
  });

  // Cancel Button
  form.querySelector('#btn-cancel-policy').onclick = () => form.remove();

  // Form Submit
  const submitBtn = form.querySelector('button[type="submit"]');
  let isSavingKey = false;

  form.onsubmit = async (event) => {
    event.preventDefault();
    if (isSavingKey) return; // Anti-spam lock

    const activeTab = form.querySelector('.mode-tab.active').dataset.mode;
    let finalRestrictions;
    if (activeTab === 'raw') {
      try {
        finalRestrictions = JSON.parse(form.querySelector('#policy-raw-json').value);
      } catch (err) {
        form.querySelector('.form-error').textContent = `Invalid JSON: ${err.message}`;
        return;
      }
    } else {
      syncToRawJson();
      finalRestrictions = state;
    }

    const keyName = form.querySelector('#policy-key-name').value.trim();
    const isActive = Number(form.querySelector('#policy-key-active').value);

    isSavingKey = true;
    submitBtn.disabled = true;
    const originalBtnHtml = submitBtn.innerHTML;
    submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving Key Policy...';
    form.querySelector('.form-error').textContent = '';

    try {
      const endpoint = isNew ? '/api/keys' : `/api/keys/${item.id}`;
      const method = isNew ? 'POST' : 'PUT';
      const body = { name: keyName, isActive: isActive, restrictions: finalRestrictions };
      const response = await fetch(`${apiBase}${endpoint}`, {
        method,
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || `${response.status} ${response.statusText}`);
      await renderView('keys');
    } catch (err) {
      form.querySelector('.form-error').textContent = err.message;
    } finally {
      isSavingKey = false;
      if (document.body.contains(form)) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalBtnHtml;
      }
    }
  };
}

function createForm(name) {
  const fields = {
    providers: [['provider', 'Provider alias', 'text', true], ['name', 'Connection name', 'text', true], ['email', 'Account email', 'email', false], ['data', 'Connection data JSON', 'textarea', true]],
    orchestrator: [['name', 'Combo name', 'text', true], ['strategy', 'Strategy (fallback, round-robin, sticky, fusion)', 'text', true], ['models', 'Models JSON array (e.g. ["gpt-4o","claude-3-5-sonnet"])', 'textarea', true]],
    keys: [['name', 'Key name', 'text', true], ['key', 'API key (optional)', 'text', false], ['restrictions', 'Restrictions JSON', 'textarea', false]],
    pools: [['data', 'Pool data JSON', 'textarea', true]],
    aliases: [['alias', 'Client model alias', 'text', true], ['target', 'Target model', 'text', true]]
  }[name];
  if (!fields) return '';
  return `<form class="inline-form" data-create="${name}">${fields.map(([key, label, type, required]) => `<label>${escapeHtml(label)}${type === 'textarea' ? `<textarea name="${key}" ${required ? 'required' : ''} rows="3"></textarea>` : `<input name="${key}" type="${type}" ${required ? 'required' : ''} />`}</label>`).join('')}<div class="form-actions"><button class="solid-button" type="submit">Save to SQLite</button><button class="cancel-button" type="button">Cancel</button></div><p class="form-error" role="alert"></p></form>`;
}

async function submitCreate(form) {
  const name = form.dataset.create;
  const values = Object.fromEntries(new FormData(form).entries());
  let parsedRestrictions = undefined;
  if (values.restrictions) {
    try { parsedRestrictions = JSON.parse(values.restrictions); } catch {}
  }
  const body = name === 'providers' ? { provider: values.provider, name: values.name, email: values.email, authType: 'apikey', data: values.data } : name === 'orchestrator' ? { name: values.name, strategy: values.strategy, models: values.models } : name === 'keys' ? { name: values.name, key: values.key, restrictions: parsedRestrictions } : name === 'aliases' ? { alias: values.alias, target: values.target } : { data: values.data };
  const endpoint = name === 'providers' ? '/api/providers' : name === 'orchestrator' ? '/api/combos' : name === 'keys' ? '/api/keys' : name === 'aliases' ? '/api/model-aliases' : '/api/proxy-pools';
  const response = await fetch(`${apiBase}${endpoint}`, { method: 'POST', headers: { ...getHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const resText = await response.text();
  let payload = {};
  try { payload = resText ? JSON.parse(resText) : {}; } catch { payload = { error: resText }; }
  if (!response.ok) throw new Error(payload.error || payload.message || resText || `${response.status} ${response.statusText}`);
}

function bindCreateForm(name) {
  const actionBtn = document.querySelector('#generic-action-button') || document.querySelector('#generic-action');
  if (!actionBtn) return;
  actionBtn.onclick = () => {
    if (name === 'keys') {
      openCreateKeyModal();
      return;
    }
    if (name === 'providers') {
      openProviderModal('openai');
      return;
    }
    if (name === 'aliases') {
      openCreateAliasModal();
      return;
    }
    if (name === 'orchestrator') {
      openCreateComboModal();
      return;
    }
    const existing = document.querySelector(`[data-create="${name}"]`);
    if (existing) existing.remove();
    content.insertAdjacentHTML('afterbegin', createForm(name));
    const form = document.querySelector(`[data-create="${name}"]`);
    if (!form) return;
    const submitBtn = form.querySelector('button[type="submit"]');
    const cancelBtn = form.querySelector('.cancel-button');
    let isSaving = false;

    cancelBtn.onclick = () => {
      if (isSaving) return;
      form.remove();
    };

    form.onsubmit = async (event) => {
      event.preventDefault();
      if (isSaving) return; // Anti-spam lock

      isSaving = true;
      submitBtn.disabled = true;
      cancelBtn.disabled = true;
      const originalBtnHtml = submitBtn.innerHTML;
      submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving to SQLite...';
      form.querySelector('.form-error').textContent = '';

      try {
        await submitCreate(form);
        await renderView(name);
      } catch (error) {
        form.querySelector('.form-error').textContent = error.message;
      } finally {
        isSaving = false;
        if (document.body.contains(form)) {
          submitBtn.disabled = false;
          submitBtn.innerHTML = originalBtnHtml;
          cancelBtn.disabled = false;
        }
      }
    };
  };
}

function comboBuilderForm(combo = {}, isNew = false, allActiveModels = []) {
  const models = parseComboModels(combo.models);
  const strategy = combo.strategy || 'fallback';
  const name = combo.name || '';

  return `
    <form class="inline-form policy-builder-form" id="combo-builder-form" data-combo-id="${escapeHtml(combo.id || '')}" style="max-width:740px;">
      <div class="form-head">
        <div>
          <span class="kicker">ORCHESTRATION / ROUTING PIPELINE</span>
          <h2>${isNew ? 'Create New Route Combo' : `Edit Flow: ${escapeHtml(name || combo.id)}`}</h2>
        </div>
        <div class="mode-tabs">
          <button type="button" class="mode-tab active" data-combo-mode="visual">Visual Pipeline</button>
          <button type="button" class="mode-tab" data-combo-mode="raw">Raw JSON</button>
        </div>
      </div>

      <div class="form-grid-2">
        <label>
          Combo Route Name
          <input name="name" id="combo-name-input" value="${escapeHtml(name)}" placeholder="e.g. smart-fallback, claude-tier" required />
        </label>
        <label>
          Routing Strategy
          <select name="strategy" id="combo-strategy-select">
            <option value="fallback" ${strategy === 'fallback' ? 'selected' : ''}>Fallback (Failover sequence on error/429)</option>
            <option value="round-robin" ${strategy === 'round-robin' ? 'selected' : ''}>Round-Robin (Equal load distribution)</option>
            <option value="sticky" ${strategy === 'sticky' ? 'selected' : ''}>Sticky (Consistent model per client session)</option>
            <option value="fusion" ${strategy === 'fusion' ? 'selected' : ''}>Fusion (Speculative multi-model)</option>
          </select>
        </label>
      </div>

      <!-- VISUAL PIPELINE BUILDER -->
      <div id="combo-visual-surface">
        <div class="builder-section">
          <div class="section-title">
            <div style="display:flex; justify-content:space-between; align-items:center;">
              <strong>Model Pipeline Sequence</strong>
              <span id="combo-step-count" class="table-badge active" style="font-size:8.5px;">${models.length} MODEL(S) IN PIPELINE</span>
            </div>
            <small>Requests evaluate models in order from Step 1 downwards. Use ▲ / ▼ to adjust priority.</small>
          </div>

          <!-- REORDERABLE PIPELINE STEPS LIST -->
          <div id="combo-pipeline-steps" style="display:grid; gap:5px; margin:6px 0;"></div>

          <!-- ADD MODEL BAR -->
          <div style="margin-top:8px; background:#080b10; border:1px solid var(--line-subtle); border-radius:6px; padding:10px;">
            <span class="quick-add-label" style="display:block; margin-bottom:6px;">QUICK ADD FROM ACTIVE NODES:</span>
            <div class="quick-chips" style="display:flex; flex-wrap:wrap; gap:4px; max-height:100px; overflow-y:auto; margin-bottom:8px;">
              ${allActiveModels.map((m) => `
                <button type="button" class="preset-chip" data-add-combo-model="${escapeHtml(m)}">
                  + ${escapeHtml(m)}
                </button>
              `).join('')}
            </div>
            <div class="custom-input-row">
              <input type="text" id="custom-combo-model-input" placeholder="Type custom model identifier..." />
              <button type="button" class="secondary-button" id="btn-add-custom-combo-step">+ Add Step</button>
            </div>
          </div>
        </div>
      </div>

      <!-- RAW JSON SURFACE -->
      <div id="combo-raw-surface" class="hidden">
        <label>
          Models Array JSON
          <textarea id="combo-raw-json" rows="8" style="font-family:var(--mono); font-size:11px;">${escapeHtml(JSON.stringify(models, null, 2))}</textarea>
        </label>
      </div>

      <div class="form-actions">
        <button class="solid-button" type="submit">Save Route Combo</button>
        <button class="cancel-button" type="button" id="btn-cancel-combo">Cancel</button>
      </div>
      <p class="form-error" role="alert"></p>
    </form>
  `;
}

function setupComboBuilderInteractions(combo = {}, isNew = false) {
  const form = document.querySelector('#combo-builder-form');
  if (!form) return;

  let pipeline = parseComboModels(combo.models);

  function syncToRawJson() {
    form.querySelector('#combo-raw-json').value = JSON.stringify(pipeline, null, 2);
    const countBadge = form.querySelector('#combo-step-count');
    if (countBadge) countBadge.textContent = `${pipeline.length} MODEL(S) IN PIPELINE`;
  }

  function renderPipelineSteps() {
    const container = form.querySelector('#combo-pipeline-steps');
    if (!container) return;

    if (pipeline.length === 0) {
      container.innerHTML = `
        <div style="text-align:center; padding:16px; color:var(--muted); font-size:11px; border:1px dashed var(--line); border-radius:5px;">
          Pipeline is empty. Click a model from below or type a custom model ID to add the first step.
        </div>
      `;
    } else {
      container.innerHTML = pipeline.map((model, idx) => `
        <div class="combo-step-row" style="display:flex; align-items:center; justify-content:space-between; background:#05070a; border:1px solid var(--line); border-radius:5px; padding:6px 10px; gap:8px;">
          <div style="display:flex; align-items:center; gap:8px; min-width:0; flex:1;">
            <span style="font:700 9px var(--mono); color:var(--lime); background:var(--lime-dim); border:1px solid rgba(200,255,99,0.3); border-radius:3px; padding:1px 5px;">STEP ${idx + 1}</span>
            <code class="model-id-code" style="font-size:11px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${escapeHtml(model)}</code>
          </div>
          <div style="display:flex; align-items:center; gap:4px;">
            <button type="button" class="reorder-btn" data-step-up="${idx}" ${idx === 0 ? 'disabled style="opacity:0.2;"' : ''} title="Move Up" style="font-size:9px; padding:2px 4px;">▲</button>
            <button type="button" class="reorder-btn" data-step-down="${idx}" ${idx === pipeline.length - 1 ? 'disabled style="opacity:0.2;"' : ''} title="Move Down" style="font-size:9px; padding:2px 4px;">▼</button>
            <button type="button" class="remove-chip" data-step-remove="${idx}" title="Remove Step" style="font-size:14px; margin-left:4px; padding:0 4px;">&times;</button>
          </div>
        </div>
      `).join('');
    }

    syncToRawJson();
    bindStepButtons();
  }

  function bindStepButtons() {
    form.querySelectorAll('[data-step-up]').forEach((btn) => {
      btn.onclick = () => {
        const i = Number(btn.dataset.stepUp);
        if (i > 0) {
          const temp = pipeline[i];
          pipeline[i] = pipeline[i - 1];
          pipeline[i - 1] = temp;
          renderPipelineSteps();
        }
      };
    });

    form.querySelectorAll('[data-step-down]').forEach((btn) => {
      btn.onclick = () => {
        const i = Number(btn.dataset.stepDown);
        if (i < pipeline.length - 1) {
          const temp = pipeline[i];
          pipeline[i] = pipeline[i + 1];
          pipeline[i + 1] = temp;
          renderPipelineSteps();
        }
      };
    });

    form.querySelectorAll('[data-step-remove]').forEach((btn) => {
      btn.onclick = () => {
        const i = Number(btn.dataset.stepRemove);
        pipeline.splice(i, 1);
        renderPipelineSteps();
      };
    });
  }

  // Quick Add model chips
  form.querySelectorAll('[data-add-combo-model]').forEach((btn) => {
    btn.onclick = () => {
      const m = btn.dataset.addComboModel;
      pipeline.push(m);
      renderPipelineSteps();
    };
  });

  // Custom step add
  const customIn = form.querySelector('#custom-combo-model-input');
  const addCustomBtn = form.querySelector('#btn-add-custom-combo-step');
  if (customIn && addCustomBtn) {
    const addCustom = () => {
      const v = customIn.value.trim();
      if (v) {
        pipeline.push(v);
        customIn.value = '';
        renderPipelineSteps();
      }
    };
    addCustomBtn.onclick = addCustom;
    customIn.onkeydown = (e) => { if (e.key === 'Enter') { e.preventDefault(); addCustom(); } };
  }

  // Mode Switcher (Visual vs Raw JSON)
  form.querySelectorAll('.mode-tab').forEach((tab) => {
    tab.onclick = () => {
      form.querySelectorAll('.mode-tab').forEach((t) => t.classList.remove('active'));
      tab.classList.add('active');
      const isVisual = tab.dataset.comboMode === 'visual';
      form.querySelector('#combo-visual-surface').classList.toggle('hidden', !isVisual);
      form.querySelector('#combo-raw-surface').classList.toggle('hidden', isVisual);
      if (isVisual) {
        try {
          const raw = JSON.parse(form.querySelector('#combo-raw-json').value);
          pipeline = Array.isArray(raw) ? raw : [String(raw)];
          renderPipelineSteps();
        } catch {}
      } else {
        syncToRawJson();
      }
    };
  });

  form.querySelector('#btn-cancel-combo').onclick = () => form.remove();

  // Form Submit
  const submitBtn = form.querySelector('button[type="submit"]');
  let isSavingCombo = false;

  form.onsubmit = async (event) => {
    event.preventDefault();
    if (isSavingCombo) return; // Anti-spam lock

    const activeTab = form.querySelector('.mode-tab.active').dataset.comboMode;
    let finalModels;
    if (activeTab === 'raw') {
      try {
        finalModels = JSON.parse(form.querySelector('#combo-raw-json').value);
      } catch (err) {
        form.querySelector('.form-error').textContent = `Invalid JSON: ${err.message}`;
        return;
      }
    } else {
      finalModels = pipeline;
    }

    if (!Array.isArray(finalModels) || finalModels.length === 0) {
      form.querySelector('.form-error').textContent = 'Pipeline must contain at least 1 model step.';
      return;
    }

    const comboName = form.querySelector('#combo-name-input').value.trim();
    const strategy = form.querySelector('#combo-strategy-select').value;
    const body = { name: comboName, strategy, models: JSON.stringify(finalModels) };

    isSavingCombo = true;
    submitBtn.disabled = true;
    const originalBtnHtml = submitBtn.innerHTML;
    submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving Route Combo...';
    form.querySelector('.form-error').textContent = '';

    try {
      const endpoint = isNew ? '/api/combos' : `/api/combos/${combo.id}`;
      const method = isNew ? 'POST' : 'PUT';
      const response = await fetch(`${apiBase}${endpoint}`, {
        method,
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      const resData = await response.json();
      if (!response.ok) throw new Error(resData.error || `${response.status} ${response.statusText}`);
      await renderView('orchestrator');
    } catch (err) {
      form.querySelector('.form-error').textContent = err.message;
    } finally {
      isSavingCombo = false;
      if (document.body.contains(form)) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalBtnHtml;
      }
    }
  };

  renderPipelineSteps();
}

function openCreateComboModal(comboId = null) {
  Promise.all([
    request('/api/combos').catch(() => ({ combos: [] })),
    request('/api/providers').catch(() => ({ connections: [] })),
    request('/models').catch(() => ({ data: [] }))
  ]).then(([comboPayload, provPayload, modelPayload]) => {
    const { allActiveModels } = getActiveProviderModels(provPayload.connections || [], modelPayload.data || []);
    let combo = { name: '', strategy: 'fallback', models: '[]' };
    let isNew = true;

    if (comboId) {
      const found = (comboPayload.combos || []).find((c) => c.id === comboId);
      if (found) {
        combo = found;
        isNew = false;
      }
    }

    const existing = document.querySelector('#combo-builder-form');
    if (existing) existing.remove();

    content.insertAdjacentHTML('afterbegin', comboBuilderForm(combo, isNew, allActiveModels));
    setupComboBuilderInteractions(combo, isNew);
  });
}

function bindComboEditors() {
  document.querySelectorAll('[data-edit-combo]').forEach((btn) => {
    btn.onclick = () => {
      openCreateComboModal(btn.dataset.editCombo);
    };
  });
}

function openCreateAliasModal(editAlias = '', editTarget = '') {
  Promise.all([
    request('/api/providers').catch(() => ({ connections: [] })),
    request('/models').catch(() => ({ data: [] })),
    request('/api/provider-nodes').catch(() => ({ nodes: [] })),
    request('/api/custom-models').catch(() => ({ customModels: [] }))
  ]).then(([provPayload, modelPayload, nodesPayload, customPayload]) => {
    const { allActiveModels } = getActiveProviderModels(provPayload.connections || [], modelPayload.data || []);
    const existing = document.querySelector('[data-create="aliases"]');
    if (existing) existing.remove();

    const isEdit = Boolean(editAlias);
    const formHtml = `
      <form class="inline-form" data-create="aliases" style="max-width:540px; padding:16px; background:#080b10; border:1px solid var(--line); border-radius:8px; margin-bottom:14px;">
        <div class="form-head" style="border-bottom:1px solid var(--line); padding-bottom:8px; margin-bottom:12px; display:flex; justify-content:space-between; align-items:center;">
          <div>
            <span class="kicker">MODEL ROUTING MAPPER</span>
            <h2 style="font-size:14px; margin:2px 0 0;">${isEdit ? `Edit Alias: ${escapeHtml(editAlias)}` : 'Create New Model Alias'}</h2>
          </div>
          <button class="cancel-button" type="button" id="btn-close-alias-modal" style="padding:2px 6px;">&times;</button>
        </div>
        <div style="display:grid; gap:10px;">
          <label style="font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Client Model Name (Incoming Alias)
            <input name="alias" id="alias-input" value="${escapeHtml(editAlias)}" placeholder="e.g. gpt-4o, claude-3-5-sonnet" ${isEdit ? 'readonly style="opacity:0.7;"' : 'required'} style="background:#05070a; border:1px solid var(--line); padding:7px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
          </label>
          <label style="font-size:10px; font-family:var(--mono); color:var(--muted); text-transform:uppercase;">
            Target Upstream Route
            <input name="target" id="target-model-input" value="${escapeHtml(editTarget)}" placeholder="e.g. antigravity/gemini-3.7-flash-high, deepseek-chat" list="alias-models-datalist" required style="background:#05070a; border:1px solid var(--line); padding:7px 10px; font:11px var(--mono); color:var(--text); border-radius:5px;" />
            <datalist id="alias-models-datalist">
              ${allActiveModels.map((m) => `<option value="${escapeHtml(m)}"></option>`).join('')}
            </datalist>
          </label>
        </div>

        <div class="form-actions" style="margin-top:12px; display:flex; gap:8px;">
          <button class="solid-button" type="submit">${isEdit ? 'Save Alias Mapping' : 'Create Alias'}</button>
          <button class="cancel-button" type="button" id="btn-cancel-alias-form">Cancel</button>
        </div>
        <p class="form-error" role="alert" style="margin-top:6px; color:var(--danger); font-size:11px;"></p>
      </form>
    `;

    content.insertAdjacentHTML('afterbegin', formHtml);
    const form = document.querySelector('[data-create="aliases"]');
    if (!form) return;
    
    const closeHandler = () => form.remove();
    form.querySelector('#btn-close-alias-modal')?.addEventListener('click', closeHandler);
    form.querySelector('#btn-cancel-alias-form')?.addEventListener('click', closeHandler);

    form.onsubmit = async (event) => {
      event.preventDefault();
      const submitBtn = form.querySelector('button[type="submit"]');
      submitBtn.disabled = true;
      const origText = submitBtn.innerHTML;
      submitBtn.innerHTML = '<span class="spinner-icon"></span> Saving...';
      try {
        const values = Object.fromEntries(new FormData(form).entries());
        await fetch(`${apiBase}/api/model-aliases`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({ alias: values.alias.trim(), target: values.target.trim() })
        });
        form.remove();
        await renderView('aliases');
      } catch (error) {
        form.querySelector('.form-error').textContent = error.message;
      } finally {
        submitBtn.disabled = false;
        submitBtn.innerHTML = origText;
      }
    };
  });
}

function bindAliasDeckActions() {
  const searchInput = document.querySelector('#alias-search-input');
  if (searchInput) {
    searchInput.oninput = () => {
      aliasSearchQuery = searchInput.value;
      aliasCurrentPage = 1;
      content.innerHTML = renderAliases();
      bindAliasDeckActions();
      // refocus search
      const reSearch = document.querySelector('#alias-search-input');
      if (reSearch) {
        reSearch.focus();
        reSearch.setSelectionRange(reSearch.value.length, reSearch.value.length);
      }
    };
  }

  document.querySelectorAll('[data-filter-prov]').forEach((chip) => {
    chip.onclick = () => {
      aliasProviderFilter = chip.dataset.filterProv;
      aliasCurrentPage = 1;
      content.innerHTML = renderAliases();
      bindAliasDeckActions();
    };
  });

  const pageSizeSelect = document.querySelector('#alias-page-size-select');
  if (pageSizeSelect) {
    pageSizeSelect.onchange = () => {
      aliasPageSize = Number(pageSizeSelect.value) || 25;
      aliasCurrentPage = 1;
      content.innerHTML = renderAliases();
      bindAliasDeckActions();
    };
  }

  const prevBtn = document.querySelector('#btn-alias-prev');
  if (prevBtn) {
    prevBtn.onclick = () => {
      if (aliasCurrentPage > 1) {
        aliasCurrentPage--;
        content.innerHTML = renderAliases();
        bindAliasDeckActions();
      }
    };
  }

  const nextBtn = document.querySelector('#btn-alias-next');
  if (nextBtn) {
    nextBtn.onclick = () => {
      aliasCurrentPage++;
      content.innerHTML = renderAliases();
      bindAliasDeckActions();
    };
  }

  const createBtn = document.querySelector('#btn-open-create-alias');
  if (createBtn) {
    createBtn.onclick = () => openCreateAliasModal();
  }

  // Clear all aliases batch button
  const clearAllBtn = document.querySelector('#btn-clear-all-aliases');
  if (clearAllBtn) {
    clearAllBtn.onclick = async () => {
      const entries = Object.entries(cachedAliasesPayload.aliases || {});
      if (entries.length === 0) return;
      const confirmed = await showConfirmModal({
        title: 'Delete All Model Aliases',
        kicker: 'DISCIPLINED PREFIX ROUTING',
        message: `Are you sure you want to delete all ${entries.length} model aliases? This will enforce that all model calls require an explicit provider prefix (e.g. ag/..., codex/...), and only Combos will be available without a prefix.`,
        confirmText: 'Delete All Aliases',
        danger: true
      });
      if (!confirmed) return;
      clearAllBtn.disabled = true;
      clearAllBtn.innerHTML = '<span class="spinner-icon"></span> Deleting...';
      try {
        await Promise.all(entries.map(([alias]) => fetch(`${apiBase}/api/model-aliases/${encodeURIComponent(alias)}`, {
          method: 'DELETE',
          headers: getHeaders()
        })));
        showToast(`All ${entries.length} aliases deleted successfully!`, 'success');
        cachedAliasesPayload = { aliases: {} };
        await renderView('aliases');
      } catch (err) {
        showToast(`Failed to clear aliases: ${err.message}`, 'error');
      }
    };
  }
  document.querySelectorAll('[data-edit-alias]').forEach((btn) => {
    btn.onclick = () => {
      const alias = btn.dataset.editAlias;
      const target = btn.dataset.target;
      openCreateAliasModal(alias, target);
    };
  });

  // Delete alias
  document.querySelectorAll('[data-delete-alias]').forEach((btn) => {
    btn.onclick = async () => {
      const alias = btn.dataset.deleteAlias;
      const confirmed = await showConfirmModal({
        title: 'Delete Model Alias',
        kicker: 'DELETE ALIAS',
        message: `Are you sure you want to delete client model alias "${alias}"?`,
        confirmText: 'Delete Alias',
        danger: true
      });
      if (!confirmed) return;
      try {
        await fetch(`${apiBase}/api/model-aliases/${encodeURIComponent(alias)}`, {
          method: 'DELETE',
          headers: getHeaders()
        });
        showToast(`Alias "${alias}" deleted`, 'info');
        await renderView('aliases');
      } catch (err) {
        showToast(`Failed to delete alias: ${err.message}`, 'error');
      }
    };
  });
  // Test live alias
  document.querySelectorAll('[data-test-alias]').forEach((btn) => {
    btn.onclick = async () => {
      const alias = btn.dataset.testAlias;
      btn.className = 'model-test-btn testing';
      btn.textContent = 'Testing...';
      const start = Date.now();
      try {
        const res = await fetch(`${apiBase}/chat/completions`, {
          method: 'POST',
          headers: { ...headers, 'Content-Type': 'application/json' },
          body: JSON.stringify({
            model: alias,
            messages: [{ role: 'user', content: 'hi' }],
            stream: false,
            max_tokens: 5
          })
        });
        const latency = Date.now() - start;
        if (res.ok) {
          btn.className = 'model-test-btn ok';
          btn.textContent = `OK (${latency}ms)`;
        } else {
          btn.className = 'model-test-btn error';
          btn.textContent = `Err: ${res.status}`;
        }
      } catch (err) {
        btn.className = 'model-test-btn error';
        btn.textContent = 'Net Err';
      }
    };
  });
}
function openCreateKeyModal() {
  Promise.all([
    request('/api/providers').catch(() => ({ connections: [] })),
    request('/models').catch(() => ({ data: [] })),
    request('/api/provider-nodes').catch(() => ({ nodes: [] })),
    request('/api/custom-models').catch(() => ({ customModels: [] }))
  ]).then(([provPayload, modelPayload, nodesPayload, customPayload]) => {
    const providers = provPayload.connections || [];
    const models = modelPayload.data || [];
    const existingForm = document.querySelector('#key-policy-form');
    if (existingForm) existingForm.remove();
    content.insertAdjacentHTML('afterbegin', keyPolicyForm({ name: '', isActive: 1, restrictions: '{}' }, true, providers, models, nodesPayload.nodes || [], customPayload.customModels || []));
    setupPolicyBuilderInteractions({ name: '', isActive: 1, restrictions: '{}' }, true);
  });
}

async function waitForVercelDeployJob(jobId, statusContainer) {
  const render = (job) => {
    const done = Number(job.completed || 0) + Number(job.failed || 0);
    const total = Math.max(Number(job.total || 1), 1);
    const percent = Math.min(100, Math.round((done / total) * 100));
    statusContainer.innerHTML = `
      <div class="deploy-status-box deploy-progress-box">
        <div class="deploy-progress-head"><strong>Vercel ${escapeHtml(job.status || 'running')}</strong><span>${done} / ${total}</span></div>
        <div class="deploy-progress-track"><div class="deploy-progress-fill" style="width:${percent}%;"></div></div>
        <small>${escapeHtml(job.currentProject || 'Preparing next project')} ${job.failed ? `· ${job.failed} failed` : ''}</small>
      </div>
    `;
  };

  while (true) {
    const response = await fetch(`${apiBase}/api/proxy-pools/vercel-deploy/jobs/${encodeURIComponent(jobId)}`, { headers: getHeaders() });
    const job = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(job.error?.message || job.error || 'Unable to read deployment job');
    render(job);
    if (job.status === 'completed') return;
    if (job.status === 'failed' || job.status === 'cancelled') {
      throw new Error(job.lastError || `Vercel deployment job ${job.status}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
}

function bindDeployButtons() {
  document.querySelectorAll('[data-deploy]').forEach((button) => {
    button.onclick = () => {
      const type = button.dataset.deploy;
      const slot = document.querySelector('#deploy-form-slot');
      if (!slot) return;

      let formBody = '';
      if (type === 'custom') {
        formBody = `
          <form class="inline-form deploy-form" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:14px; margin-top:8px;">
            <span class="kicker">NEW CUSTOM PROXY (HTTP / SOCKS5)</span>
            <div style="display:grid; gap:8px; margin-top:6px;">
              <label>Proxy Pool Name<input name="name" placeholder="e.g. US Residential Proxy" required /></label>
              <label>Proxy URL<input name="proxyUrl" placeholder="http://user:pass@host:port or socks5://127.0.0.1:1080" required /></label>
              <label>Bypass Domains (No Proxy)<input name="noProxy" placeholder="localhost, 127.0.0.1 (optional)" /></label>
            </div>
            <div class="form-actions" style="margin-top:10px;">
              <button class="solid-button" type="submit">Save Proxy Pool</button>
              <button class="cancel-button" type="button">Cancel</button>
            </div>
            <div class="deploy-status-container"></div>
            <p class="form-error" role="alert"></p>
          </form>
        `;
      } else if (type === 'cloudflare') {
        formBody = `
          <form class="inline-form deploy-form" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:14px; margin-top:8px;">
            <span class="kicker">CLOUDFLARE WORKERS RELAY DEPLOY</span>
            <div style="display:grid; gap:8px; margin-top:6px;">
              <label>Cloudflare Account ID<input name="accountId" placeholder="32-character hexadecimal Account ID" required /></label>
              <label>Cloudflare API Token<input name="apiToken" type="password" placeholder="API Token with Workers Scripts:Edit permission" required /></label>
              <label>Worker Project Name<input name="projectName" placeholder="relay-worker-1 (optional)" /></label>
            </div>
            <div class="form-actions" style="margin-top:10px;">
              <button class="solid-button" type="submit">Deploy to Cloudflare</button>
              <button class="cancel-button" type="button">Cancel</button>
            </div>
            <div class="deploy-status-container"></div>
            <p class="form-error" role="alert"></p>
          </form>
        `;
      } else if (type === 'deno') {
        formBody = `
          <form class="inline-form deploy-form" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:14px; margin-top:8px;">
            <span class="kicker">DENO DEPLOY RELAY</span>
            <div style="display:grid; gap:8px; margin-top:6px;">
              <label>Deno Access Token<input name="apiToken" type="password" placeholder="Deno Deploy Access Token" required /></label>
              <label>Project Name (Optional)<input name="projectName" placeholder="relay-deno-1" /></label>
            </div>
            <div class="form-actions" style="margin-top:10px;">
              <button class="solid-button" type="submit">Deploy to Deno</button>
              <button class="cancel-button" type="button">Cancel</button>
            </div>
            <div class="deploy-status-container"></div>
            <p class="form-error" role="alert"></p>
          </form>
        `;
      } else if (type === 'vercel') {
        formBody = `
          <form class="inline-form deploy-form" style="background:#080b10; border:1px solid var(--line); border-radius:6px; padding:14px; margin-top:8px;">
            <span class="kicker">VERCEL RELAY DEPLOY</span>
            <div style="display:grid; gap:8px; margin-top:6px;">
              <label>Vercel Token<input name="apiToken" type="password" placeholder="Vercel personal access token" required /></label>
              <label>Project Name (Optional)<input name="projectName" placeholder="zyrouter-relay-1" /></label>
              <label>Deploy Mode<select name="mode"><option value="single">Single project</option><option value="bulk">Bulk projects</option></select></label>
              <label>Project Count<input name="count" type="number" min="1" max="50" value="1" /></label>
              <label>Delay Mode<select name="delayMode"><option value="fixed">Fixed delay</option><option value="random">Random delay</option></select></label>
              <div style="display:grid; grid-template-columns:1fr 1fr; gap:8px;">
                <label>Min Delay (sec)<input name="delayMinSeconds" type="number" min="0" max="3600" value="5" /></label>
                <label>Max Delay (sec)<input name="delayMaxSeconds" type="number" min="0" max="3600" value="10" /></label>
              </div>
            </div>
            <div class="form-actions" style="margin-top:10px;">
              <button class="solid-button" type="submit">Deploy to Vercel</button>
              <button class="cancel-button" type="button">Cancel</button>
            </div>
            <div class="deploy-status-container"></div>
            <p class="form-error" role="alert"></p>
          </form>
        `;
      }
      slot.innerHTML = `<div class="deploy-modal-backdrop"><div class="deploy-modal" role="dialog" aria-modal="true">${formBody}</div></div>`;
      const form = slot.querySelector('form');
      if (!form) return;
      const cancelBtn = form.querySelector('.cancel-button');
      const submitBtn = form.querySelector('button[type="submit"]');
      const statusContainer = form.querySelector('.deploy-status-container');
      const errEl = form.querySelector('.form-error');
      let isSubmitting = false;

      cancelBtn.onclick = () => {
        if (isSubmitting) return;
        slot.innerHTML = '';
      };

      form.onsubmit = async (event) => {
        event.preventDefault();
        if (isSubmitting) return; // Anti-spam lock

        isSubmitting = true;
        // Read form values before disabling controls; disabled inputs are
        // intentionally omitted by FormData.
        const values = Object.fromEntries(new FormData(form).entries());
        submitBtn.disabled = true;
        cancelBtn.disabled = true;
        form.querySelectorAll('input').forEach((input) => { input.disabled = true; });
        errEl.textContent = '';

        const originalBtnHtml = submitBtn.innerHTML;
        const actionLabel = type === 'custom' ? 'Saving Proxy Pool' : `Deploying to ${type.toUpperCase()}`;
        submitBtn.innerHTML = `<span class="spinner-icon"></span> ${escapeHtml(actionLabel)}...`;

        statusContainer.innerHTML = `
          <div class="deploy-status-box">
            <span class="spinner-icon"></span>
            <span>${type === 'custom' ? 'Validating and saving proxy pool...' : `Provisioning and compiling serverless relay on <strong>${type.toUpperCase()}</strong>... Please wait (~5-15s).`}</span>
          </div>
        `;

        try {
          let endpoint = `/api/proxy-pools/${type}-deploy`;
          let bodyPayload = { ...values };
          if (type === 'vercel') {
            bodyPayload.vercelToken = values.apiToken || values.vercelToken || values.token || '';
            bodyPayload.apiToken = bodyPayload.vercelToken;
            if (values.mode === 'bulk' || Number(values.count || 1) > 1) {
              endpoint = '/api/proxy-pools/vercel-deploy/jobs';
              bodyPayload = {
                vercelToken: bodyPayload.vercelToken,
                projectName: values.projectName || '',
                count: Math.min(Math.max(Number(values.count || 1), 1), 50),
                delayMode: values.delayMode === 'random' ? 'random' : 'fixed',
                delayMinSeconds: Math.max(Number(values.delayMinSeconds || 0), 0),
                delayMaxSeconds: Math.max(Number(values.delayMaxSeconds || values.delayMinSeconds || 0), 0)
              };
            }
          } else if (type === 'deno') {
            bodyPayload.denoToken = values.apiToken || values.denoToken || values.token || '';
            bodyPayload.apiToken = bodyPayload.denoToken;
          } else if (type === 'custom') {
            endpoint = '/api/proxy-pools';
            bodyPayload = { name: values.name, proxyUrl: values.proxyUrl, noProxy: values.noProxy, type: 'http', isActive: 1 };
          }
          let response = await fetch(`${apiBase}${endpoint}`, {
            method: 'POST',
            headers: { ...getHeaders(), 'Content-Type': 'application/json' },
            body: JSON.stringify(bodyPayload)
          });
          if (!response.ok && endpoint.startsWith('/api/')) {
            const fallbackEndpoint = endpoint.replace('/api/', '/');
            const altRes = await fetch(`${apiBase}${fallbackEndpoint}`, {
              method: 'POST',
              headers: { ...headers, 'Content-Type': 'application/json' },
              body: JSON.stringify(bodyPayload)
            }).catch(() => null);
            if (altRes && altRes.ok) response = altRes;
          }

          const resText = await response.text();
          let payload = {};
          try {
            payload = JSON.parse(resText);
          } catch {
            payload = { error: resText || `${response.status} ${response.statusText}` };
          }
          let errorMsg = '';
          if (payload) {
            if (typeof payload.error === 'string') errorMsg = payload.error;
            else if (payload.error && typeof payload.error.message === 'string') errorMsg = payload.error.message;
            else if (typeof payload.message === 'string') errorMsg = payload.message;
            else if (typeof payload.error === 'object') errorMsg = JSON.stringify(payload.error);
          }
          if (!errorMsg) errorMsg = resText || `${response.status} ${response.statusText}`;
          if (!response.ok) throw new Error(errorMsg);

          if (type === 'vercel' && endpoint.endsWith('/jobs')) {
            const jobId = payload.id || payload.jobId;
            if (!jobId) throw new Error('Vercel deployment job did not return an ID');
            await waitForVercelDeployJob(jobId, statusContainer);
          }
          
          statusContainer.innerHTML = '';
          slot.innerHTML = `<p class="result-line" style="color:var(--lime); padding:10px 0;"><span class="icon-indicator" style="color:var(--lime);">&#10003;</span> ${type === 'vercel' && endpoint.endsWith('/jobs') ? 'Bulk Vercel deployment completed.' : 'Proxy pool deployed and saved.'}</p>`;
          showToast(type === 'vercel' && endpoint.endsWith('/jobs') ? 'Bulk Vercel deployment completed.' : 'Proxy pool deployed and saved.', 'success');
          await renderView('pools');
        } catch (error) {
          statusContainer.innerHTML = '';
          errEl.textContent = error.message;
        } finally {
          isSubmitting = false;
          submitBtn.disabled = false;
          submitBtn.innerHTML = originalBtnHtml;
          cancelBtn.disabled = false;
          form.querySelectorAll('input').forEach((input) => { input.disabled = false; });
        }
      };
    };
  });
  // Individual Proxy Test Action with instant in-place DOM feedback
  document.querySelectorAll('[data-test-pool]').forEach((btn) => {
    btn.onclick = async () => {
      const poolId = btn.dataset.testPool;
      const statusBadge = document.querySelector(`#badge-status-${CSS.escape(poolId)}`);
      const activeBadge = document.querySelector(`#badge-active-${CSS.escape(poolId)}`);
      
      btn.disabled = true;
      const origText = btn.innerHTML;
      btn.innerHTML = '<span class="spinner-icon"></span> Testing...';
      if (statusBadge) {
        statusBadge.className = 'table-badge inactive';
        statusBadge.textContent = 'TESTING...';
      }

      try {
        const res = await fetch(`${apiBase}/api/proxy-pools/${encodeURIComponent(poolId)}/test`, {
          method: 'POST',
          headers: getHeaders()
        });
        const data = await res.json().catch(() => ({}));
        if (data.ok) {
          if (statusBadge) {
            statusBadge.className = 'table-badge active';
            statusBadge.textContent = 'PASSED';
          }
          if (activeBadge) {
            activeBadge.className = 'table-badge active';
            activeBadge.textContent = 'ACTIVE';
          }
          showToast(`Proxy test passed (${data.elapsedMs || 0}ms)`, 'success');
        } else {
          if (statusBadge) {
            statusBadge.className = 'table-badge error';
            statusBadge.textContent = 'ERROR';
          }
          showToast(`Proxy test failed: ${data.error || 'Connection error'}`, 'error');
        }
      } catch (err) {
        if (statusBadge) {
          statusBadge.className = 'table-badge error';
          statusBadge.textContent = 'ERROR';
        }
        showToast(`Proxy test error: ${err.message}`, 'error');
      } finally {
        btn.disabled = false;
        btn.innerHTML = origText;
      }
    };
  });

  // High-Concurrency Parallel Batch Health Check (10-worker pool like 9router ori)
  const testAllBtn = document.querySelector('#btn-test-all-pools');
  if (testAllBtn) {
    testAllBtn.onclick = async () => {
      const allPools = cachedPoolsPayload.proxyPools || [];
      if (allPools.length === 0) return;

      testAllBtn.disabled = true;
      const origHtml = testAllBtn.innerHTML;
      testAllBtn.innerHTML = '<span class="spinner-icon"></span> Health Checking...';

      const progressWrap = document.querySelector('#pool-health-progress-wrap');
      const progressLabel = document.querySelector('#pool-health-progress-label');
      const progressStats = document.querySelector('#pool-health-progress-stats');
      const progressFill = document.querySelector('#pool-health-progress-fill');

      if (progressWrap) progressWrap.style.display = 'block';

      let passed = 0;
      let failed = 0;
      let completed = 0;
      const total = allPools.length;
      const CONCURRENCY = 10;
      const queue = [...allPools];

      const updateProgress = () => {
        const pct = Math.round((completed / total) * 100);
        if (progressFill) progressFill.style.width = `${pct}%`;
        if (progressStats) progressStats.textContent = `${completed} / ${total} (${passed} passed, ${failed} failed)`;
        if (progressLabel) progressLabel.textContent = `⚡ Running 10-Worker Parallel Health Check (${pct}%)...`;
      };
      updateProgress();

      const worker = async () => {
        while (queue.length > 0) {
          const pool = queue.shift();
          if (!pool) break;
          const poolId = pool.id;
          const statusBadge = document.querySelector(`#badge-status-${CSS.escape(poolId)}`);
          const activeBadge = document.querySelector(`#badge-active-${CSS.escape(poolId)}`);
          const rowBtn = document.querySelector(`#btn-test-pool-${CSS.escape(poolId)}`);

          if (rowBtn) {
            rowBtn.disabled = true;
            rowBtn.innerHTML = '<span class="spinner-icon"></span>';
          }
          if (statusBadge) {
            statusBadge.className = 'table-badge inactive';
            statusBadge.textContent = 'TESTING...';
          }

          try {
            const res = await fetch(`${apiBase}/api/proxy-pools/${encodeURIComponent(poolId)}/test`, {
              method: 'POST',
              headers: getHeaders()
            });
            const data = await res.json().catch(() => ({}));
            if (data.ok) {
              passed++;
              pool.testStatus = 'active';
              pool.isActive = true;
              if (statusBadge) {
                statusBadge.className = 'table-badge active';
                statusBadge.textContent = 'PASSED';
              }
              if (activeBadge) {
                activeBadge.className = 'table-badge active';
                activeBadge.textContent = 'ACTIVE';
              }
            } else {
              failed++;
              pool.testStatus = 'error';
              if (statusBadge) {
                statusBadge.className = 'table-badge error';
                statusBadge.textContent = 'ERROR';
              }
            }
          } catch {
            failed++;
            pool.testStatus = 'error';
            if (statusBadge) {
              statusBadge.className = 'table-badge error';
              statusBadge.textContent = 'ERROR';
            }
          } finally {
            completed++;
            if (rowBtn) {
              rowBtn.disabled = false;
              rowBtn.innerHTML = 'Test';
            }
            updateProgress();
          }
        }
      };

      await Promise.all(Array.from({ length: Math.min(CONCURRENCY, total) }, worker));
      showToast(`Health check complete: ${passed} passed, ${failed} failed`, passed > 0 ? 'success' : 'error');
      
      if (progressLabel) progressLabel.textContent = `✓ Health Check Finished (${passed} passed, ${failed} failed)`;
      setTimeout(() => {
        if (progressWrap) progressWrap.style.display = 'none';
      }, 4000);

      testAllBtn.disabled = false;
      testAllBtn.innerHTML = origHtml;
    };
  }

  // Search input binding
  const searchInput = document.querySelector('#pool-search-input');
  if (searchInput) {
    searchInput.oninput = () => {
      poolSearchQuery = searchInput.value;
      poolCurrentPage = 1;
      content.innerHTML = renderPools();
      bindDeployButtons();
      // Maintain focus at end of search input
      const newSearch = document.querySelector('#pool-search-input');
      if (newSearch) {
        newSearch.focus();
        newSearch.setSelectionRange(newSearch.value.length, newSearch.value.length);
      }
    };
  }

  // Type filter chips
  document.querySelectorAll('[data-filter-pool-type]').forEach((chip) => {
    chip.onclick = () => {
      poolTypeFilter = chip.dataset.filterPoolType;
      poolCurrentPage = 1;
      content.innerHTML = renderPools();
      bindDeployButtons();
    };
  });

  // Page size selector
  const pageSizeSelect = document.querySelector('#pool-page-size-select');
  if (pageSizeSelect) {
    pageSizeSelect.onchange = () => {
      poolPageSize = Number(pageSizeSelect.value) || 25;
      poolCurrentPage = 1;
      content.innerHTML = renderPools();
      bindDeployButtons();
    };
  }

  // Pagination Prev / Next buttons
  const prevBtn = document.querySelector('#btn-pool-prev');
  if (prevBtn) {
    prevBtn.onclick = () => {
      if (poolCurrentPage > 1) {
        poolCurrentPage--;
        content.innerHTML = renderPools();
        bindDeployButtons();
      }
    };
  }

  const nextBtn = document.querySelector('#btn-pool-next');
  if (nextBtn) {
    nextBtn.onclick = () => {
      poolCurrentPage++;
      content.innerHTML = renderPools();
      bindDeployButtons();
    };
  }
}

function bindSettings() {
  const save = document.querySelector('#save-settings');
  const json = document.querySelector('#settings-json');
  const result = document.querySelector('#settings-result');
  const toggles = document.querySelectorAll('[data-setting]');
  const exportBtn = document.querySelector('#export-settings');
  const importInput = document.querySelector('#import-settings');
  const changePassForm = document.querySelector('#change-password-form');
  const exportDbBtn = document.querySelector('#btn-export-database');
  const importDbInput = document.querySelector('#import-database-file');
  const dbStatusEl = document.querySelector('#db-backup-status');

  // Full Database Backup Export
  if (exportDbBtn) {
    exportDbBtn.onclick = async () => {
      exportDbBtn.disabled = true;
      const origHtml = exportDbBtn.innerHTML;
      exportDbBtn.innerHTML = '<span class="spinner-icon"></span> Exporting database...';
      try {
        const res = await fetch(`${apiBase}/api/settings/database`, {
          headers: getHeaders()
        });
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `zyrouter-full-backup-${new Date().toISOString().slice(0, 10)}.json`;
        link.click();
        URL.revokeObjectURL(url);
        showToast('Full database backup downloaded successfully!', 'success');
      } catch (err) {
        showToast(`Export failed: ${err.message}`, 'error');
      } finally {
        exportDbBtn.disabled = false;
        exportDbBtn.innerHTML = origHtml;
      }
    };
  }

  // Full Database Backup Restore
  if (importDbInput) {
    importDbInput.onchange = (event) => {
      const file = event.target.files?.[0];
      importDbInput.value = '';
      if (!file) return;
      const reader = new FileReader();
      reader.onload = async () => {
        try {
          const parsed = JSON.parse(reader.result);
          const confirmed = await showConfirmModal({
            title: 'Restore Full Database',
            kicker: 'DATABASE RESTORE',
            message: 'Restoring this backup will replace all active provider connections, API keys, proxy pools, combos, and custom models with the data from this backup file. Do you want to proceed?',
            confirmText: 'Restore Database',
            danger: true
          });
          if (!confirmed) return;

          if (dbStatusEl) {
            dbStatusEl.style.color = 'var(--lime)';
            dbStatusEl.innerHTML = '<span class="spinner-icon"></span> Restoring database...';
          }

          const res = await fetch(`${apiBase}/api/settings/database`, {
            method: 'POST',
            headers: { ...getHeaders(), 'Content-Type': 'application/json' },
            body: JSON.stringify(parsed)
          });
          const resText = await res.text();
          let data = {};
          try { data = JSON.parse(resText); } catch {}
          if (!res.ok) throw new Error(data.error || resText || `${res.status} ${res.statusText}`);

          showToast('Database restored successfully! Reloading...', 'success');
          setTimeout(() => window.location.reload(), 1000);
        } catch (err) {
          showToast(`Restore failed: ${err.message}`, 'error');
          if (dbStatusEl) {
            dbStatusEl.style.color = 'var(--danger)';
            dbStatusEl.textContent = `Restore failed: ${err.message}`;
          }
        }
      };
      reader.readAsText(file);
    };
  }

  // Change Password
  if (changePassForm) {
    changePassForm.onsubmit = async (e) => {
      e.preventDefault();
      const submitBtn = changePassForm.querySelector('#btn-change-password-submit');
      const statusEl = changePassForm.querySelector('#change-password-status');
      const values = Object.fromEntries(new FormData(changePassForm).entries());
      submitBtn.disabled = true;
      const origText = submitBtn.innerHTML;
      submitBtn.innerHTML = '<span class="spinner-icon"></span> Updating...';
      if (statusEl) statusEl.textContent = '';
      try {
        const res = await fetch(`${apiBase}/api/auth/change-password`, {
          method: 'POST',
          headers: { ...getHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({
            currentPassword: values.currentPassword,
            newPassword: values.newPassword
          })
        });
        const resText = await res.text();
        let data = {};
        try { data = JSON.parse(resText); } catch {}
        if (!res.ok) throw new Error(data.error || resText || 'Failed to update password');
        showToast('Dashboard password updated successfully!', 'success');
        changePassForm.reset();
        if (statusEl) {
          statusEl.style.color = 'var(--lime)';
          statusEl.textContent = '✓ Password updated';
          setTimeout(() => { statusEl.textContent = ''; }, 3000);
        }
      } catch (err) {
        showToast(err.message, 'error');
        if (statusEl) {
          statusEl.style.color = 'var(--danger)';
          statusEl.textContent = err.message;
        }
      } finally {
        submitBtn.disabled = false;
        submitBtn.innerHTML = origText;
      }
    };
  }

  if (exportBtn) {
    exportBtn.onclick = () => {
      const blob = new Blob([json.value], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `zyrouter-settings-${new Date().toISOString().slice(0, 10)}.json`;
      link.click();
      URL.revokeObjectURL(url);
    };
  }

  if (importInput) {
    importInput.onchange = (event) => {
      const file = event.target.files?.[0];
      importInput.value = '';
      if (!file) return;
      const reader = new FileReader();
      reader.onload = async () => {
        try {
          const parsed = JSON.parse(reader.result);
          json.value = JSON.stringify(parsed, null, 2);
          result.textContent = 'Settings JSON loaded into editor. Click Save to persist.';
        } catch (error) {
          result.textContent = `Import failed: ${error.message}`;
        }
      };
      reader.readAsText(file);
    };
  }

  if (save && json && result) {
    save.addEventListener('click', async () => {
      try {
        const payload = JSON.parse(json.value);
        const response = await fetch(`${apiBase}/api/settings`, { method: 'PUT', headers: { ...getHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
        const resText = await response.text();
        let body = {};
        try { body = JSON.parse(resText); } catch {}
        if (!response.ok) throw new Error(body.error || resText || `${response.status} ${response.statusText}`);
        result.textContent = 'Settings saved to SQLite database.';
        showToast('Settings saved successfully!', 'success');
      } catch (error) {
        result.textContent = `Save failed: ${error.message}`;
        showToast(`Save failed: ${error.message}`, 'error');
      }
    });
  }

  toggles.forEach((checkbox) => {
    checkbox.addEventListener('change', () => {
      try {
        const payload = JSON.parse(json.value);
        payload[checkbox.dataset.setting] = checkbox.checked;
        json.value = JSON.stringify(payload, null, 2);
      } catch (error) {
        result.textContent = `Unable to update settings draft: ${error.message}`;
      }
    });
  });
}

let consoleIsPaused = false;
let consoleAutoScroll = true;
function bindLogStream() {
  const terminalBody = document.querySelector('#console-terminal-body');
  const scrollContainer = document.querySelector('#console-scroll-container');
  const activeContainer = document.querySelector('#console-active-container');
  const inflightBadge = document.querySelector('#console-inflight-badge');
  const bufferCountEl = document.querySelector('#console-buffer-count');
  const footerCountEl = document.querySelector('#console-footer-count');
  const statusText = document.querySelector('#console-stream-status-text');
  const pauseBtn = document.querySelector('#btn-console-pause');
  const clearBtn = document.querySelector('#btn-console-clear');
  const searchInput = document.querySelector('#console-search-input');
  const filterBtns = document.querySelectorAll('#console-status-filter .console-tab-btn');
  const providerSelect = document.querySelector('#console-provider-filter');

  let activeFilter = 'all';
  let activeProvider = 'all';
  let activeSearch = '';

  const filterRows = () => {
    if (!terminalBody) return;
    let visibleCount = 0;
    const allRows = terminalBody.querySelectorAll('.console-row');
    allRows.forEach((row) => {
      const status = row.dataset.status || '';
      const provider = (row.dataset.provider || '').toLowerCase();
      const query = row.dataset.query || '';

      let matchesFilter = true;
      if (activeFilter === '2xx') matchesFilter = status.startsWith('2');
      else if (activeFilter === '4xx/5xx') matchesFilter = status.startsWith('4') || status.startsWith('5') || status === 'error';

      let matchesProv = true;
      if (activeProvider !== 'all') {
        if (activeProvider === 'custom') matchesProv = provider.startsWith('openai-compatible') || provider.startsWith('anthropic-compatible');
        else matchesProv = provider.includes(activeProvider);
      }

      const matchesSearch = !activeSearch || query.includes(activeSearch);

      const isVisible = matchesFilter && matchesProv && matchesSearch;
      row.style.display = isVisible ? '' : 'none';
      if (isVisible) visibleCount++;
    });

    if (footerCountEl) {
      footerCountEl.innerHTML = `Showing <strong>${visibleCount}</strong> of ${allRows.length}`;
    }
  };

  if (searchInput) {
    searchInput.oninput = () => {
      activeSearch = searchInput.value.trim().toLowerCase();
      filterRows();
    };
  }

  filterBtns.forEach((btn) => {
    btn.onclick = () => {
      filterBtns.forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      activeFilter = btn.dataset.filter;
      filterRows();
    };
  });

  if (providerSelect) {
    providerSelect.onchange = () => {
      activeProvider = providerSelect.value;
      filterRows();
    };
  }

  if (pauseBtn) {
    pauseBtn.onclick = () => {
      consoleIsPaused = !consoleIsPaused;
      pauseBtn.innerHTML = consoleIsPaused
        ? '<span style="color:var(--lime); font-size:10px; margin-right:4px;">▶</span> Resume Stream'
        : '<span style="color:#f59e0b; font-size:10px; margin-right:4px;">⏸</span> Pause Stream';
      if (statusText) statusText.textContent = consoleIsPaused ? 'Stream Paused' : 'Live Request Stream';
    };
  }

  if (clearBtn && terminalBody) {
    clearBtn.onclick = () => {
      terminalBody.innerHTML = `
        <tr id="console-empty-row">
          <td colspan="7" style="text-align: center; color: var(--muted); padding: 48px 14px; font-style: italic;">
            Terminal buffer cleared. Listening on live stream...
          </td>
        </tr>
      `;
      if (bufferCountEl) bufferCountEl.textContent = '0 reqs';
      if (footerCountEl) footerCountEl.innerHTML = 'Showing <strong>0</strong> of 0';
    };
  }

  // Row Click -> Payload Inspector Drawer
  const attachRowClicks = () => {
    if (!terminalBody) return;
    terminalBody.querySelectorAll('.console-row').forEach((row) => {
      row.onclick = () => {
        let reqData = {};
        try { reqData = JSON.parse(row.dataset.req || '{}'); } catch {}
        openPayloadInspectorDrawer(reqData);
      };
    });
  };
  attachRowClicks();

  // Subscribe to live SSE usage stream
  const streamPath = '/api/usage/stream';
  const fallbackStreamPath = '/translator/console-logs/stream';
  startStream(streamPath, (payload) => {
    // 1. In-flight active requests
    const activeList = Array.isArray(payload.activeRequests) ? payload.activeRequests : [];
    const totalActive = activeList.reduce((sum, item) => sum + (item.count || 0), 0);
    if (inflightBadge) {
      inflightBadge.textContent = `${totalActive} IN-FLIGHT`;
      inflightBadge.className = `table-badge ${totalActive > 0 ? 'active' : ''}`;
    }

    // 2. Append completed requests to stream table & live Usage Ledger
    if (!consoleIsPaused && Array.isArray(payload.recentRequests) && payload.recentRequests.length > 0) {
      const topReq = payload.recentRequests[0];

      // Real-time incremental update for Usage Ledger (#usage)
      if (topReq) {
        if (!cachedUsagePayload) cachedUsagePayload = {};
        if (!Array.isArray(cachedUsagePayload.recentRequests)) cachedUsagePayload.recentRequests = [];

        cachedUsagePayload.recentRequests.unshift(topReq);
        cachedUsagePayload.totalRequests = (cachedUsagePayload.totalRequests || 0) + 1;
        const pToks = Number(topReq.promptTokens || 0);
        const cToks = Number(topReq.completionTokens || 0);
        cachedUsagePayload.totalTokens = (cachedUsagePayload.totalTokens || 0) + pToks + cToks;
        if (topReq.cost) cachedUsagePayload.totalCost = (cachedUsagePayload.totalCost || 0) + topReq.cost;

        // Targeted DOM updates if user is looking at #usage
        const totalReqEl = document.querySelector('#usage-total-requests');
        if (totalReqEl) totalReqEl.textContent = Number(cachedUsagePayload.totalRequests).toLocaleString();

        const totalTokEl = document.querySelector('#usage-total-tokens');
        if (totalTokEl) {
          const tt = cachedUsagePayload.totalTokens;
          totalTokEl.textContent = formatTokenCount(tt);
          totalTokEl.title = `${Number(tt).toLocaleString('en-US')} tokens`;
        }

        const totalCostEl = document.querySelector('#usage-total-cost');
        if (totalCostEl) totalCostEl.textContent = `$${Number(cachedUsagePayload.totalCost || 0).toFixed(4)}`;

        const countBadge = document.querySelector('#usage-recent-count-badge');
        if (countBadge) countBadge.textContent = `${cachedUsagePayload.recentRequests.length} RECENT`;

        const tbody = document.querySelector('#usage-recent-tbody');
        if (tbody) {
          const emptyTr = tbody.querySelector('td[colspan]');
          if (emptyTr) emptyTr.closest('tr').remove();

          const timeStr = topReq.timestamp ? new Date(topReq.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
          const dateStr = topReq.timestamp ? new Date(topReq.timestamp).toLocaleDateString([], { month: 'short', day: 'numeric' }) : 'Sep 1';
          const toks = pToks + cToks;
          const isErr = topReq.status === 'error' || String(topReq.status).startsWith('4') || String(topReq.status).startsWith('5');
          const statusText = isErr ? String(topReq.status || 'error') : 'success';

          const tr = document.createElement('tr');
          tr.innerHTML = `
            <td>
              <div style="font-family:var(--mono); line-height:1.2;">
                <span style="font-size:10px; color:var(--text-bright); font-weight:500;">${escapeHtml(timeStr)}</span>
                <small style="display:block; font-size:8px; color:var(--muted);">${escapeHtml(dateStr)}</small>
              </div>
            </td>
            <td><code class="model-id-code" style="font-size:10.5px;">${escapeHtml(topReq.model || '--')}</code></td>
            <td><span style="color:var(--muted); font-size:10.5px;">${escapeHtml(topReq.provider || '--')}</span></td>
            <td><span class="table-cell-mono" style="font-size:10px; color:var(--text);">${toks > 0 ? `${toks.toLocaleString()}t` : '--'}</span></td>
            <td style="text-align: right;"><span class="table-badge ${isErr ? 'inactive' : 'active'}" style="font-size:7.5px;">${escapeHtml(statusText)}</span></td>
          `;
          tbody.insertBefore(tr, tbody.firstChild);

          // Keep table view capped at page size
          while (tbody.children.length > usageRecentPageSize) {
            tbody.removeChild(tbody.lastChild);
          }
        }
      }
      if (terminalBody) {
        const statusCode = topReq.status || 200;
        const statusDotColor = isErr ? '#ef4444' : '#22c55e';
          const totalTokens = (topReq.promptTokens || 0) + (topReq.completionTokens || 0);
          const latencySec = topReq.durationMs ? (topReq.durationMs / 1000).toFixed(2) + 's' : (topReq.latency ? topReq.latency : '1.78s');
          const latNum = parseFloat(latencySec) || 0;
          const latClass = latNum > 15 ? 'slow' : (latNum > 4 ? 'med' : 'fast');
          const reqId = topReq.id || `${Date.now()}-${topReq.model || 'chat'}`;
          const provName = (topReq.provider || 'gateway').toLowerCase();
          const costSavings = topReq.savings ? `$${topReq.savings.toFixed(2)}` : '$0.00';

          const row = document.createElement('tr');
          row.className = 'console-row';
          row.dataset.status = String(statusCode);
          row.dataset.provider = provName;
          row.dataset.query = `${reqId} ${provName} ${topReq.model || ''} ${statusCode}`.toLowerCase();
          row.dataset.req = JSON.stringify(topReq);
          row.innerHTML = `
            <td>
              <span style="display:inline-flex; align-items:center; gap:6px; font-weight:700; color:${statusDotColor};">
                <span style="font-size:9px;">●</span> ${escapeHtml(String(statusCode))}
              </span>
            </td>
            <td>
              <span class="method-tag post">POST</span>
              <span style="color:#94a3b8; font-size:11px;">${escapeHtml(reqId)}</span>
            </td>
            <td>
              <div style="display:flex; align-items:center; gap:8px;">
                <span class="prov-pill ${escapeHtml(provName.startsWith('openai-compatible') ? 'custom' : provName)}">
                  ${escapeHtml(provName)}
                </span>
                <strong style="color:var(--text-bright); font-size:11.5px;">${escapeHtml(topReq.model || 'model')}</strong>
              </div>
            </td>
            <td style="text-align: right; color:#e2e8f0;">
              ${totalTokens > 0 ? `${totalTokens.toLocaleString()} tok` : '--'}
            </td>
            <td style="text-align: right;">
              <span class="latency-badge ${latClass}">${escapeHtml(latencySec)}</span>
            </td>
            <td style="text-align: right; color:#facc15;">
              ${escapeHtml(costSavings)}
            </td>
            <td style="text-align: right; color:#64748b; font-size:10px;">
              ${escapeHtml(timeStr)}
            </td>
          `;

          terminalBody.insertBefore(row, terminalBody.firstChild);

          // Keep buffer capped at 100 entries
          while (terminalBody.children.length > 100) {
            terminalBody.removeChild(terminalBody.lastChild);
          }

          if (bufferCountEl) bufferCountEl.textContent = `${terminalBody.children.length} reqs`;
          if (scrollContainer) scrollContainer.scrollTop = 0;
          attachRowClicks();
        filterRows();
      }
    }
  });
}

function openPayloadInspectorDrawer(reqData = {}) {
  const existing = document.querySelector('#payload-drawer-overlay');
  if (existing) existing.remove();

  const overlay = document.createElement('div');
  overlay.id = 'payload-drawer-overlay';
  overlay.className = 'payload-drawer-backdrop';
  overlay.innerHTML = `
    <div class="payload-drawer">
      <div class="payload-drawer-head">
        <div>
          <span class="kicker" style="font-size:8.5px; color:var(--lime);">PAYLOAD INSPECTOR</span>
          <h3 style="font-size:14px; margin:2px 0 0; color:var(--text-bright); font-family:var(--mono);">
            ${escapeHtml(reqData.id || reqData.model || 'Request Details')}
          </h3>
        </div>
        <button type="button" class="cancel-button" id="btn-close-payload-drawer" style="font-size:16px; padding:2px 8px;">&times;</button>
      </div>

      <div class="payload-drawer-body">
        <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:8px;">
          <div class="card" style="padding:10px;">
            <span class="kicker">STATUS</span>
            <div style="font-size:13px; font-weight:700; color:${(reqData.status || 200) < 400 ? 'var(--lime)' : 'var(--danger)'};">
              ${escapeHtml(String(reqData.status || 200))}
            </div>
          </div>
          <div class="card" style="padding:10px;">
            <span class="kicker">TOKENS</span>
            <div style="font-size:13px; font-weight:700; color:var(--text-bright);">
              ${((reqData.promptTokens || 0) + (reqData.completionTokens || 0)).toLocaleString()}t
            </div>
          </div>
          <div class="card" style="padding:10px;">
            <span class="kicker">LATENCY</span>
            <div style="font-size:13px; font-weight:700; color:#38bdf8;">
              ${escapeHtml(reqData.durationMs ? (reqData.durationMs / 1000).toFixed(2) + 's' : (reqData.latency || '--'))}
            </div>
          </div>
        </div>

        <div class="card" style="padding:12px;">
          <span class="kicker">ROUTING &amp; PROXY TRACE</span>
          <div style="display:grid; gap:6px; margin-top:6px; font-family:var(--mono); font-size:11px;">
            <div><span style="color:var(--muted);">Provider:</span> <strong style="color:var(--text-bright);">${escapeHtml(reqData.provider || '--')}</strong></div>
            <div><span style="color:var(--muted);">Model:</span> <strong style="color:var(--lime);">${escapeHtml(reqData.model || '--')}</strong></div>
            <div><span style="color:var(--muted);">Account:</span> <span style="color:var(--text-bright);">${escapeHtml(reqData.account || reqData.connectionId || 'Primary Node')}</span></div>
            <div><span style="color:var(--muted);">Outbound Proxy:</span> <span class="table-badge ${reqData.proxy && reqData.proxy !== 'Direct' ? 'purple' : ''}" style="font-size:8.5px;">${escapeHtml(reqData.proxy || 'Direct')}</span></div>
            <div><span style="color:var(--muted);">Strategy:</span> <span style="color:#a1a1aa; text-transform:uppercase;">${escapeHtml(reqData.strategy || 'fallback')}</span></div>
          </div>
        </div>

        <div class="card" style="padding:12px;">
          <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px;">
            <span class="kicker">RAW EVENT JSON</span>
            <button class="secondary-button" id="btn-copy-raw-json" style="font-size:9.5px; padding:2px 6px;">Copy JSON</button>
          </div>
          <pre style="background:#05070a; border:1px solid var(--line); padding:10px; border-radius:6px; font-family:var(--mono); font-size:10px; color:#cbd5e1; overflow-x:auto; max-height:240px; margin:0;">${escapeHtml(JSON.stringify(reqData, null, 2))}</pre>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(overlay);
  overlay.onclick = (e) => {
    if (e.target === overlay) overlay.remove();
  };
  const closeBtn = overlay.querySelector('#btn-close-payload-drawer');
  if (closeBtn) closeBtn.onclick = () => overlay.remove();

  const copyBtn = overlay.querySelector('#btn-copy-raw-json');
  if (copyBtn) {
    copyBtn.onclick = async () => {
            await copyText(JSON.stringify(reqData, null, 2));
      copyBtn.textContent = 'Copied!';
      setTimeout(() => { copyBtn.textContent = 'Copy JSON'; }, 1500);
    };
  }
}

function setView(name) {
  const baseName = name.startsWith('provider/') ? 'providers' : name;
  const data = views[baseName] || views.overview;
  document.querySelectorAll('.dock-item').forEach((item) => item.classList.toggle('active', item.dataset.view === baseName));
  overview.classList.toggle('hidden', name !== 'overview');
  generic.classList.toggle('hidden', name === 'overview');
  breadcrumb.textContent = name.startsWith('provider/') ? `PROVIDER / ${name.split('/')[1].toUpperCase()}` : data[0];
  if (name !== 'overview') {
    document.querySelector('#generic-title').innerHTML = `${data[1]}<em>_</em>`;
    document.querySelector('#generic-subtitle').textContent = data[2];
    document.querySelector('#generic-action').textContent = data[3];
    renderView(name);
  } else {
    loadOverview();
  }
  window.location.hash = name;
}
let isLoadingOverview = false;
let meshProviderSignature = '';
let meshProviderSyncTimer = null;
async function loadOverview() {
  if (isLoadingOverview) return;
  if (!hasDashboardAccess()) {
    renderFullLoginGate();
    return;
  }
  isLoadingOverview = true;
  try {
    const [providerPayload, usagePayload, nodesPayload] = await Promise.all([
      request('/api/providers').catch(() => ({ connections: [] })),
      request('/api/usage/stats?period=all&days=all').catch(() => ({})),
      request('/api/provider-nodes').catch(() => ({ nodes: [] }))
    ]);
    const providers = providerPayload.connections || [];
    const customNodes = nodesPayload.nodes || [];
    const customNodeIds = new Set(customNodes.map((n) => String(n.id || '').toLowerCase()));
    // Hide stale custom connections whose provider node was already deleted.
    const meshProviders = providers.filter((connection) => {
      const provider = String(connection.provider || '').toLowerCase();
      const isCustom = provider.startsWith('openai-compatible-') || provider.startsWith('anthropic-compatible-');
      return !isCustom || customNodeIds.has(provider);
    });
    const nodeNameMap = new Map();
    customNodes.forEach((n) => {
      const name = n.name || n.prefix || 'Custom Node';
      nodeNameMap.set(n.id.toLowerCase(), name);
      if (n.prefix) nodeNameMap.set(n.prefix.toLowerCase(), name);
    });
    const active = Array.isArray(usagePayload.activeRequests) ? usagePayload.activeRequests.reduce((sum, item) => sum + (item.count || 0), 0) : 0;
    const promptTokens = Number(usagePayload.promptTokens ?? usagePayload.totalPromptTokens ?? 0);
    const completionTokens = Number(usagePayload.completionTokens ?? usagePayload.totalCompletionTokens ?? 0);
    const totalTokens = Number(usagePayload.totalTokens ?? (promptTokens + completionTokens) ?? 0);
    const activeConnsCount = providers.filter(isItemActive).length;
    const totalConnsCount = providers.length;

    // 1. Throughput & Tokens Card V2 (Real-time stream token rate)
    const tokRateEl = document.querySelector('#realtime-tok-rate');
    if (tokRateEl) {
      tokRateEl.textContent = active > 0 ? `${active * 128}` : '0';
    }

    // 2. Real Token Usage from SQLite
    const totalTokensEl = document.querySelector('#cost-saved-value');
    if (totalTokensEl) {
      totalTokensEl.textContent = formatTokenCount(totalTokens);
      totalTokensEl.title = `${totalTokens.toLocaleString('en-US')} tokens`;
    }
    const tokenRatioEl = document.querySelector('#tokens-in-out-ratio');
    if (tokenRatioEl) {
      tokenRatioEl.textContent = promptTokens > 0 ? `• ${Math.round(promptTokens/1000)}k in / ${Math.round(completionTokens/1000)}k out` : '• 0 tokens';
    }

    // 3. Upstream Nodes Card V2
    const nodesCountEl = document.querySelector('#providers-count-v2');
    if (nodesCountEl) {
      nodesCountEl.textContent = `${activeConnsCount}/${totalConnsCount}`;
    }
    const activeAccountsTxt = document.querySelector('#active-accounts-txt');
    if (activeAccountsTxt) {
      activeAccountsTxt.textContent = `${activeConnsCount} Active Account(s)`;
    }

    const navBadge = document.querySelector('#nav-badge-providers');
    if (navBadge) navBadge.textContent = totalConnsCount;

    // 4. Routing Success Rate Card V2
    const recentReqs = Array.isArray(usagePayload.recentRequests) ? usagePayload.recentRequests : [];
    const totalReqsNum = Number(usagePayload.totalRequests ?? recentReqs.length ?? 0);
    const sloRateEl = document.querySelector('#slo-rate-value');
    const sloTotalEl = document.querySelector('#slo-total-reqs');
    const sloTotalSub = document.querySelector('#slo-total-sub');
    const sloLatencyEl = document.querySelector('#slo-avg-latency');

    if (sloTotalEl) sloTotalEl.textContent = `${totalReqsNum} Reqs`;
    if (sloTotalSub) sloTotalSub.textContent = `${totalReqsNum} Total`;

    if (recentReqs.length > 0) {
      const errCount = recentReqs.filter((r) => r.status === 'error' || String(r.status).startsWith('4') || String(r.status).startsWith('5')).length;
      const successRate = (((recentReqs.length - errCount) / recentReqs.length) * 100).toFixed(2);
      if (sloRateEl) sloRateEl.textContent = `${successRate}%`;

      const validDurations = recentReqs.filter(r => r.durationMs && r.durationMs > 0);
      const avgDuration = validDurations.length > 0 ? validDurations.reduce((sum, r) => sum + r.durationMs, 0) / validDurations.length : 0;
      if (sloLatencyEl) sloLatencyEl.innerHTML = avgDuration > 0 ? `&bull; ${(avgDuration / 1000).toFixed(2)}s avg` : `&bull; < 1s avg`;

      // Populate Overview Event Activity Box
      const overviewStreamBox = document.querySelector('#overview-recent-stream-box');
      if (overviewStreamBox) {
        overviewStreamBox.innerHTML = recentReqs.slice(0, 7).map((req, idx) => {
          const isErr = req.status === 'error' || String(req.status).startsWith('4') || String(req.status).startsWith('5');
          const statusCode = req.status || 200;
          const statusColor = isErr ? '#ef4444' : '#22c55e';
          const toks = (req.promptTokens || 0) + (req.completionTokens || 0);
          const pName = (req.provider || 'gateway').toLowerCase();
          const cleanP = pName.startsWith('openai-compatible') ? 'custom' : pName;
          const timeStr = req.timestamp
            ? new Date(req.timestamp).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
              })
            : '--:--:--';

          return `
            <div class="console-log-row" style="background:#05070a; border:1px solid rgba(255,255,255,0.05); border-radius:6px; padding:6px 10px; display:flex; justify-content:space-between; align-items:center; gap:8px;">
              <div style="display:flex; align-items:center; gap:6px; min-width:0; overflow:hidden;">
                <span style="color:${statusColor}; font-weight:bold; font-size:10px; font-family:var(--mono);">● ${escapeHtml(String(statusCode))}</span>
                <span class="method-tag post" style="margin:0; font-size:8px;">POST</span>
                <strong style="color:var(--text-bright); font-size:11px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${escapeHtml(req.model || 'model')}</strong>
              </div>
              <div style="display:flex; align-items:center; gap:6px; flex-shrink:0; font-family:var(--mono); font-size:9.5px;">
                <span class="prov-pill ${escapeHtml(cleanP)}" style="font-size:8.5px; padding:1px 4px;">${escapeHtml(cleanP)}</span>
                <span style="color:var(--muted);">${toks > 0 ? `${toks}t` : '--'}</span>
                <span style="color:#52525b; font-size:8.5px;">${escapeHtml(timeStr)}</span>
              </div>
            </div>
          `;
        }).join('');
      }
    } else {
      if (sloRateEl) sloRateEl.textContent = '100.00%';
      if (sloLatencyEl) sloLatencyEl.innerHTML = '&bull; 0.00s avg';
    }

    const envEl = document.querySelector('.environment');
    if (envEl) envEl.innerHTML = 'LOCAL ENGINE <i></i>';

    const connStrong = document.querySelector('.connection-strip strong');
    if (connStrong) connStrong.textContent = 'Operational • REST & SSE Connected';

    const connMsg = document.querySelector('.strip-message');
    if (connMsg) connMsg.textContent = 'REST state loaded from the configured Go engine & SQLite repository.';
    
    // Populate Live Cyber Mesh Topology Providers
    const meshProvCol = document.querySelector('#mesh-providers-col');
    const nextMeshProviderSignature = JSON.stringify({
      connections: meshProviders.map((c) => ({ id: c.id, provider: c.provider, name: c.name, isActive: c.isActive })),
      nodes: customNodes.map((n) => ({ id: n.id, name: n.name, prefix: n.prefix }))
    });
    const meshProvidersChanged = nextMeshProviderSignature !== meshProviderSignature;
    if (meshProvCol && meshProvidersChanged) {
      meshProviderSignature = nextMeshProviderSignature;
      if (!Array.isArray(meshProviders) || meshProviders.length === 0) {
        meshProvCol.innerHTML = `
          <div class="mesh-node provider-node" data-provider-id="none">
            <span class="material-symbols-outlined mesh-icon" style="color:var(--dim);">cloud_off</span>
            <div class="mesh-node-info">
              <strong>No Providers</strong>
              <small class="mesh-status-txt">Offline</small>
            </div>
          </div>
        `;
      } else {
        const provMap = new Map();
        meshProviders.forEach((c) => {
          // Check active status
          const isActive = isItemActive(c);
          if (!isActive) return;

          let rawProv = (c.provider || '').toLowerCase();
          let cleanProvKey = rawProv;
          let friendlyName = '';

          let d = {};
          try { d = typeof c.data === 'string' ? JSON.parse(c.data) : (c.data || {}); } catch {}
          const psd = d.providerSpecificData || {};
          const explicitNodeName = psd.nodeName || psd.prefix || '';

          if (nodeNameMap.has(rawProv)) {
            friendlyName = nodeNameMap.get(rawProv);
          } else if (explicitNodeName) {
            friendlyName = explicitNodeName;
            cleanProvKey = rawProv.startsWith('anthropic') ? 'anthropic' : 'openai';
          } else if (rawProv.startsWith('openai-compatible')) {
            cleanProvKey = 'openai';
            friendlyName = (c.name && c.name.length > 2) ? c.name : 'OpenAI Node';
          } else if (rawProv.startsWith('anthropic-compatible')) {
            cleanProvKey = 'anthropic';
            friendlyName = (c.name && c.name.length > 2) ? c.name : 'Anthropic Node';
          } else {
            const cat = KNOWN_PROVIDER_CATALOG.find((p) => p.id === rawProv || rawProv.startsWith(p.id));
            if (cat && cat.name) {
              friendlyName = cat.name;
              cleanProvKey = cat.id;
            } else {
              friendlyName = c.name || rawProv.toUpperCase();
            }
          }
          if (!provMap.has(rawProv)) {
            provMap.set(rawProv, {
              provId: c.provider,
              name: friendlyName,
              iconKey: cleanProvKey,
              conns: []
            });
          }
          provMap.get(rawProv).conns.push(c);
        });
        if (provMap.size === 0) {
          meshProvCol.innerHTML = `
            <div class="mesh-node provider-node" data-provider-id="none">
              <span class="material-symbols-outlined mesh-icon" style="color:var(--dim);">cloud_off</span>
              <div class="mesh-node-info">
                <strong>No Active Nodes</strong>
                <small class="mesh-status-txt">Offline</small>
              </div>
            </div>
          `;
        } else {
          meshProvCol.innerHTML = Array.from(provMap.values()).map((p) => {
            const activeCount = p.conns.filter(isItemActive).length;
            return `
              <div class="mesh-node provider-node" data-provider-id="${escapeHtml(p.provId)}" style="cursor:pointer;">
                ${renderProviderIcon(p.iconKey || p.provId)}
                <div class="mesh-node-info">
                  <strong>${escapeHtml(p.name)}</strong>
                  <small class="mesh-status-txt" style="color:${activeCount > 0 ? 'var(--lime)' : 'var(--red)'};">
                    ● ${activeCount > 0 ? `${activeCount} Active` : 'Offline'}
                  </small>
                </div>
              </div>
            `;
          }).join('');
        }

        meshProvCol.querySelectorAll('.provider-node').forEach((node) => {
          node.onclick = () => {
            const pid = node.dataset.providerId;
            if (pid && pid !== 'none') {
              window.location.hash = `provider/${pid}`;
              renderProviderDetail(pid);
            }
          };
        });
      }
      initMeshZoomPanControls();
      layoutMeshGraph();
      setTimeout(drawMeshLines, 100);
    }
  } catch (error) {
    if (error.status === 401 || (error.message && error.message.includes('401'))) {
      renderFullLoginGate();
    } else {
      console.debug('[zyrouter] overview data unavailable', error.message);
    }
  } finally {
    isLoadingOverview = false;
  }
}

const meshPositionSeeds = new Map();
function meshPositionSeed(key) {
  const normalized = String(key || 'mesh');
  if (!meshPositionSeeds.has(normalized)) {
    meshPositionSeeds.set(normalized, {
      angle: Math.random(),
      radius: 0.82 + Math.random() * 0.18
    });
  }
  return meshPositionSeeds.get(normalized);
}

function layoutMeshGraph() {
  const container = document.querySelector('#cyber-mesh-container');
  const hub = document.querySelector('#mesh-center-hub');
  if (!container || !hub) return;

  const nodes = Array.from(container.querySelectorAll('.mesh-clients-col .mesh-node, .mesh-providers-col .mesh-node'));
  const activeKeys = new Set(nodes.map((node, index) => node.dataset.providerId || node.dataset.clientId || index));
  for (const key of meshPositionSeeds.keys()) {
    if (!activeKeys.has(key)) meshPositionSeeds.delete(key);
  }
  const width = Math.max(container.clientWidth, 1);
  // Give dense graphs more vertical breathing room on small screens instead
  // of forcing a virtual 520px canvas that gets clipped by the viewport.
  const compact = width < 700;
  const height = Math.max(compact ? 390 : 360, compact
    ? 250 + Math.ceil(nodes.length / 3) * 38
    : 230 + Math.ceil(nodes.length / 4) * 24);
  container.style.height = `${height}px`;

  const hubWidth = hub.offsetWidth || 120;
  const hubHeight = hub.offsetHeight || 120;
  hub.style.left = `${width / 2 - hubWidth / 2}px`;
  hub.style.top = `${height / 2 - hubHeight / 2}px`;

  // Stable pseudo-random orbit positions keep the graph organic without jittering on refresh.
  const orbitX = Math.max(compact ? 135 : 170, width * (compact ? 0.32 : 0.36));
  const orbitY = Math.max(compact ? 145 : 125, height * (compact ? 0.37 : 0.34));
  const angleStep = (Math.PI * 2) / Math.max(nodes.length, 1);
  const positions = nodes.map((node, index) => {
    const seed = meshPositionSeed(node.dataset.providerId || node.dataset.clientId || index);
    const angle = index * angleStep - Math.PI / 2 + (seed.angle - 0.5) * 0.42;
    const radius = seed.radius;
    const nodeWidth = node.offsetWidth || 110;
    const nodeHeight = node.offsetHeight || 28;
    const x = width / 2 + Math.cos(angle) * orbitX * radius - nodeWidth / 2;
    const y = height / 2 + Math.sin(angle) * orbitY * radius - nodeHeight / 2;
    return { node, x, y, width: nodeWidth, height: nodeHeight };
  });

  const clamp = (position) => {
    position.x = Math.max(8, Math.min(width - position.width - 8, position.x));
    position.y = Math.max(8, Math.min(height - position.height - 8, position.y));
  };

  // Relax overlapping cards while keeping the stable pseudo-random graph shape.
  for (let pass = 0; pass < 12; pass += 1) {
    positions.forEach((a, i) => {
      for (let j = i + 1; j < positions.length; j += 1) {
        const b = positions[j];
        const dx = (a.x + a.width / 2) - (b.x + b.width / 2);
        const dy = (a.y + a.height / 2) - (b.y + b.height / 2);
        const overlapX = (a.width + b.width) / 2 + 8 - Math.abs(dx);
        const overlapY = (a.height + b.height) / 2 + 8 - Math.abs(dy);
        if (overlapX <= 0 || overlapY <= 0) continue;
        if (overlapX < overlapY) {
          const push = (overlapX / 2) * (dx >= 0 ? 1 : -1);
          a.x += push;
          b.x -= push;
        } else {
          const push = (overlapY / 2) * (dy >= 0 ? 1 : -1);
          a.y += push;
          b.y -= push;
        }
        clamp(a);
        clamp(b);
      }
    });
  }

  positions.forEach(({ node, x, y, width: nodeWidth, height: nodeHeight }) => {
    node.style.left = `${Math.max(8, Math.min(width - nodeWidth - 8, x))}px`;
    node.style.top = `${Math.max(8, Math.min(height - nodeHeight - 8, y))}px`;
  });
}

function getMeshNodeRect(el, container) {
  let x = 0;
  let y = 0;
  let curr = el;
  while (curr && curr !== container) {
    x += curr.offsetLeft;
    y += curr.offsetTop;
    curr = curr.offsetParent;
  }
  const w = el.offsetWidth || 0;
  const h = el.offsetHeight || 0;
  return {
    left: x,
    right: x + w,
    top: y,
    bottom: y + h,
    width: w,
    height: h,
    centerX: x + w / 2,
    centerY: y + h / 2
  };
}

function drawMeshLines() {
  const container = document.querySelector('#cyber-mesh-container');
  const svg = document.querySelector('#mesh-svg-layer');
  const hub = document.querySelector('#mesh-center-hub');
  if (!container || !svg || !hub) return;

  const w = container.offsetWidth;
  const h = container.offsetHeight;
  if (w === 0 || h === 0) return;

  svg.setAttribute('viewBox', `0 0 ${w} ${h}`);
  svg.style.width = `${w}px`;
  svg.style.height = `${h}px`;

  const hubCore = hub.querySelector('.mesh-hub-core') || hub;
  const hubRect = getMeshNodeRect(hubCore, container);

  let pathsHtml = '';

  // Connect every node to the nearest edge of the gateway hub, neuron-style.
  document.querySelectorAll('.mesh-clients-col .mesh-node, .mesh-providers-col .mesh-node').forEach((node) => {
    const n = getMeshNodeRect(node, container);
    const dx = hubRect.centerX - n.centerX;
    const dy = hubRect.centerY - n.centerY;
    const nodeScale = 1 / Math.max(
      Math.abs(dx) / Math.max(n.width / 2, 1),
      Math.abs(dy) / Math.max(n.height / 2, 1)
    );
    const hubScale = 1 / Math.max(
      Math.abs(dx) / Math.max(hubRect.width / 2, 1),
      Math.abs(dy) / Math.max(hubRect.height / 2, 1)
    );
    // Slight overlap prevents anti-aliased SVG edges from creating a visible gap.
    const overlap = 3;
    const nodeX = n.centerX + dx * (nodeScale + overlap / Math.max(Math.abs(dx), 1));
    const nodeY = n.centerY + dy * (nodeScale + overlap / Math.max(Math.abs(dy), 1));
    const hubX = hubRect.centerX - dx * (hubScale - overlap / Math.max(Math.abs(dx), 1));
    const hubY = hubRect.centerY - dy * (hubScale - overlap / Math.max(Math.abs(dy), 1));
    const c1X = nodeX + (hubX - nodeX) * 0.42;
    const c1Y = nodeY + (hubY - nodeY) * 0.08;
    const c2X = nodeX + (hubX - nodeX) * 0.58;
    const c2Y = hubY - (hubY - nodeY) * 0.08;
    const d = `M ${nodeX} ${nodeY} C ${c1X} ${c1Y}, ${c2X} ${c2Y}, ${hubX} ${hubY}`;
    const isActive = node.classList.contains('active');
    pathsHtml += `<path d="${d}" class="mesh-path-glow ${isActive ? 'active' : ''}" />`;
    pathsHtml += `<path d="${d}" class="mesh-path-base ${isActive ? 'mesh-path-laser' : ''}" />`;
  });

  svg.innerHTML = pathsHtml;
}

let meshFlashResetTimer;
function updateMeshRealtimeState(activeRequests = []) {
  const statusBadge = document.querySelector('#mesh-live-status');
  const latencyBadge = document.querySelector('#mesh-core-latency');
  const clients = Array.from(document.querySelectorAll('.mesh-clients-col .mesh-node'));
  const providers = Array.from(document.querySelectorAll('.mesh-providers-col .mesh-node'));

  if (!Array.isArray(activeRequests) || activeRequests.length === 0) {
    clients.forEach((c) => c.classList.remove('active'));
    providers.forEach((p) => p.classList.remove('active'));
    if (statusBadge) {
      statusBadge.className = 'live-chip';
      statusBadge.textContent = 'STANDBY • LISTENING';
    }
    if (latencyBadge) latencyBadge.textContent = '• Idle Gateway';
    drawMeshLines();
    return;
  }

  if (statusBadge) {
    statusBadge.className = 'live-chip active';
    statusBadge.textContent = `ROUTING ${activeRequests.length} ACTIVE REQUEST(S)`;
  }
  if (latencyBadge) latencyBadge.textContent = `• ${activeRequests.length} in-flight`;

  const activeProvIds = new Set();
  const activeClientIds = new Set();

  activeRequests.forEach((req) => {
    const prov = (req.provider || '').toLowerCase();
    const model = (req.model || '').toLowerCase();
    const clientHeader = (req.client || '').toLowerCase();
    if (prov) activeProvIds.add(prov);

    // Accurate client resolution based on client identifier, model, or provider
    if (clientHeader.includes('claude') || model.includes('claude') || prov === 'claude') activeClientIds.add('claude');
    else if (clientHeader.includes('cursor') || model.includes('cursor') || prov === 'cursor') activeClientIds.add('cursor');
    else if (clientHeader.includes('cline') || clientHeader.includes('roo') || model.includes('cline') || model.includes('roo') || prov === 'cline') activeClientIds.add('cline');
    else if (clientHeader.includes('opencode') || model.includes('opencode') || prov === 'opencode' || prov === 'opencode-go' || prov.startsWith('oc')) activeClientIds.add('opencode');
    else if (clientHeader.includes('copilot') || model.includes('copilot') || prov === 'copilot' || prov === 'github' || prov === 'codex') activeClientIds.add('copilot');
    else activeClientIds.add('opencode');
  });

  clients.forEach((c) => c.classList.toggle('active', activeClientIds.has(c.dataset.clientId)));
  providers.forEach((p) => {
    const pid = (p.dataset.providerId || '').toLowerCase();
    const isActive = activeProvIds.has(pid) || Array.from(activeProvIds).some(ap => pid.includes(ap) || ap.includes(pid));
    p.classList.toggle('active', isActive);
  });

  drawMeshLines();
}

function flashMeshOnLog(line) {
  const text = String(line || '').toLowerCase();
  if (!text.includes('/chat/completions') && !text.includes('/messages') && !text.includes('[request]')) return;

  const providers = Array.from(document.querySelectorAll('.mesh-providers-col .mesh-node'));
  const matched = providers.find((p) => text.includes(p.dataset.providerId));
  if (matched) {
    matched.classList.add('active');
    const client = document.querySelector('.mesh-clients-col .mesh-node');
    if (client) client.classList.add('active');
    drawMeshLines();

    clearTimeout(meshFlashResetTimer);
    meshFlashResetTimer = setTimeout(() => {
      updateMeshRealtimeState([]);
    }, 1600);
  }
}
let meshZoom = 1.0;
let isPanningMesh = false;
let panStartX = 0;
let panStartY = 0;
let panOffsetX = 0;
let panOffsetY = 0;
function updateMeshZoom(newZoom) {
  meshZoom = Math.min(Math.max(newZoom, 0.4), 2.5);
  const badge = document.querySelector('#mesh-zoom-level-badge');
  if (badge) badge.textContent = `${Math.round(meshZoom * 100)}%`;
  const container = document.querySelector('#cyber-mesh-container');
  if (container) {
    container.style.transform = `scale(${meshZoom}) translate(${panOffsetX}px, ${panOffsetY}px)`;
    setTimeout(drawMeshLines, 40);
  }
}

function initMeshZoomPanControls() {
  const zoomInBtn = document.querySelector('#btn-mesh-zoom-in');
  const zoomOutBtn = document.querySelector('#btn-mesh-zoom-out');
  const zoomResetBtn = document.querySelector('#btn-mesh-zoom-reset');
  const fullscreenBtn = document.querySelector('#btn-mesh-fullscreen');
  const viewport = document.querySelector('#mesh-viewport-wrapper');
  const matrixCard = document.querySelector('#route-matrix-card');

  if (zoomInBtn) zoomInBtn.onclick = (e) => { e.preventDefault(); updateMeshZoom(meshZoom + 0.15); };
  if (zoomOutBtn) zoomOutBtn.onclick = (e) => { e.preventDefault(); updateMeshZoom(meshZoom - 0.15); };
  if (zoomResetBtn) {
    zoomResetBtn.onclick = (e) => {
      e.preventDefault();
      panOffsetX = 0;
      panOffsetY = 0;
      updateMeshZoom(1.0);
    };
  }
  if (fullscreenBtn && matrixCard) {
    fullscreenBtn.onclick = () => {
      matrixCard.classList.toggle('fullscreen-mode');
      const isFull = matrixCard.classList.contains('fullscreen-mode');
      fullscreenBtn.textContent = isFull ? '✕' : '⛶';
      fullscreenBtn.classList.toggle('active', isFull);
      setTimeout(drawMeshLines, 100);
    };
  }

  // Mouse wheel zoom over viewport
  if (viewport) {
    viewport.addEventListener('wheel', (e) => {
      e.preventDefault();
      const delta = e.deltaY < 0 ? 0.08 : -0.08;
      updateMeshZoom(meshZoom + delta);
    }, { passive: false });

    // Drag / Pan support
    viewport.addEventListener('mousedown', (e) => {
      if (e.target.closest('button') || e.target.closest('.mesh-node')) return;
      isPanningMesh = true;
      panStartX = e.clientX - panOffsetX;
      panStartY = e.clientY - panOffsetY;
      viewport.style.cursor = 'grabbing';
    });

    window.addEventListener('mousemove', (e) => {
      if (!isPanningMesh) return;
      panOffsetX = e.clientX - panStartX;
      panOffsetY = e.clientY - panStartY;
      const container = document.querySelector('#cyber-mesh-container');
      if (container) {
        container.style.transform = `scale(${meshZoom}) translate(${panOffsetX}px, ${panOffsetY}px)`;
        drawMeshLines();
      }
    });

    window.addEventListener('mouseup', () => {
      if (isPanningMesh) {
        isPanningMesh = false;
        if (viewport) viewport.style.cursor = 'default';
        drawMeshLines();
      }
    });
  }
}

window.addEventListener('resize', () => {
  layoutMeshGraph();
  drawMeshLines();
});
const streamListeners = new Set();
let globalStreamController = null;
let streamReconnectTimer = null;

function ensureGlobalStream() {
  if (globalStreamController) return;
  if (!hasDashboardAccess()) return;

  globalStreamController = new AbortController();
  const streamPath = `${apiBase}/api/usage/stream`;

  fetch(streamPath, { headers: getHeaders(), signal: globalStreamController.signal }).then(async (response) => {
    if (!response.ok || !response.body) throw new Error(`${response.status} ${response.statusText}`);
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (!globalStreamController.signal.aborted) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const events = buffer.split('\n\n');
      buffer = events.pop();
      events.forEach((event) => {
        event.split('\n')
          .filter((line) => line.startsWith('data: '))
          .forEach((line) => {
            const raw = line.slice(6).trim();
            if (!raw || raw === '[DONE]' || raw === 'ping' || raw === '200 OK') return;
            let parsed = raw;
            try {
              parsed = JSON.parse(raw);
            } catch {}
            streamListeners.forEach((fn) => {
              try { fn(parsed); } catch (e) { console.debug('[zyrouter] stream callback err', e); }
            });
          });
      });
    }
  }).catch((error) => {
    if (globalStreamController && !globalStreamController.signal.aborted) {
      console.debug('[zyrouter] global stream disconnected, reconnecting in 2s...', error.message);
      globalStreamController = null;
      clearTimeout(streamReconnectTimer);
      streamReconnectTimer = setTimeout(ensureGlobalStream, 2000);
    }
  });
}

function startStream(path, onMessage) {
  if (typeof onMessage === 'function') {
    streamListeners.add(onMessage);
  }
  ensureGlobalStream();
}
document.addEventListener('click', (event) => {
  const trigger = event.target.closest('[data-view]');
  if (trigger) {
    setView(trigger.dataset.view);
  }
});
document.querySelector('.strip-action')?.addEventListener('click', (event) => event.currentTarget.closest('.connection-strip').remove());
document.querySelector('#refresh-button')?.addEventListener('click', loadOverview);
document.querySelector('.avatar')?.addEventListener('click', async () => {
  const confirmed = await showConfirmModal({
    title: 'Sign Out of Dashboard',
    kicker: 'SESSION SECURITY',
    message: 'Are you sure you want to log out of this dashboard session? You will need your dashboard password to sign in again.',
    confirmText: 'Sign Out',
    danger: false
  });
  if (confirmed) {
    await fetch(`${apiBase}/api/auth/logout`, { method: 'POST', headers: getHeaders() }).catch(() => {});
    dashboardAuthenticated = false;
    setAuthToken(null);
    // Lock the current view immediately. The delayed reload is only a
    // secondary reset; it must not leave private dashboard data visible while
    // the browser is waiting for navigation.
    if (globalStreamController) {
      globalStreamController.abort();
      globalStreamController = null;
    }
    clearTimeout(streamReconnectTimer);
    if (meshProviderSyncTimer) {
      clearInterval(meshProviderSyncTimer);
      meshProviderSyncTimer = null;
    }
    renderFullLoginGate();
    showToast('Signed out of dashboard', 'info');
    setTimeout(() => window.location.reload(), 400);
  }
});

startStream('/api/usage/stream', (payload) => {
  if (!hasDashboardAccess() || !payload) return;

  const active = Array.isArray(payload.activeRequests) ? payload.activeRequests.reduce((sum, item) => sum + (item.count || 0), 0) : 0;
  const reqVal = document.querySelector('#requests-value');
  if (reqVal) reqVal.textContent = active;
  updateMeshRealtimeState(payload.activeRequests || []);

  if (Array.isArray(payload.recentRequests) && payload.recentRequests.length > 0) {
    const topReq = payload.recentRequests[0];

    // 1. Update In-Memory Cache for Usage Ledger
    if (cachedUsagePayload) {
      if (!Array.isArray(cachedUsagePayload.recentRequests)) cachedUsagePayload.recentRequests = [];
      const topId = topReq.id || `${topReq.timestamp}_${topReq.model}`;
      const isDupe = cachedUsagePayload.recentRequests.some(r => r.id === topId || (r.timestamp === topReq.timestamp && r.model === topReq.model && r.promptTokens === topReq.promptTokens));
      if (!isDupe) {
        cachedUsagePayload.recentRequests.unshift({ ...topReq, id: topId });
        cachedUsagePayload.totalRequests = (cachedUsagePayload.totalRequests || 0) + 1;
        const pToks = Number(topReq.promptTokens || 0);
        const cToks = Number(topReq.completionTokens || 0);
        cachedUsagePayload.totalTokens = (cachedUsagePayload.totalTokens || 0) + pToks + cToks;
        if (topReq.cost) cachedUsagePayload.totalCost = (cachedUsagePayload.totalCost || 0) + topReq.cost;
      }
    }

    // 2. Real-time Live DOM Sync on #usage view (Cards, Sparklines, Table)
    const totalReqEl = document.querySelector('#usage-total-requests');
    if (totalReqEl && cachedUsagePayload) {
      totalReqEl.textContent = Number(cachedUsagePayload.totalRequests || 0).toLocaleString();
    }

    const totalTokEl = document.querySelector('#usage-total-tokens');
    if (totalTokEl && cachedUsagePayload) {
      const tt = Number(cachedUsagePayload.totalTokens || 0);
      totalTokEl.textContent = formatTokenCount(tt);
      totalTokEl.title = `${tt.toLocaleString('en-US')} tokens`;
    }

    const totalCostEl = document.querySelector('#usage-total-cost');
    if (totalCostEl && cachedUsagePayload) {
      totalCostEl.textContent = `$${Number(cachedUsagePayload.totalCost || 0).toFixed(4)}`;
    }

    const countBadge = document.querySelector('#usage-recent-count-badge');
    if (countBadge && cachedUsagePayload && cachedUsagePayload.recentRequests) {
      countBadge.textContent = `${cachedUsagePayload.recentRequests.length} RECENT`;
    }

    // Prepend new row to table live
    const tbody = document.querySelector('#usage-recent-tbody');
    if (tbody && topReq && topReq.model) {
      const emptyTr = tbody.querySelector('td[colspan]');
      if (emptyTr) emptyTr.closest('tr').remove();

      const timeStr = topReq.timestamp ? new Date(topReq.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      const dateStr = topReq.timestamp ? new Date(topReq.timestamp).toLocaleDateString([], { month: 'short', day: 'numeric' }) : 'Sep 1';
      const pToks = Number(topReq.promptTokens || 0);
      const cToks = Number(topReq.completionTokens || 0);
      const toks = pToks + cToks;
      const isErr = topReq.status === 'error' || String(topReq.status).startsWith('4') || String(topReq.status).startsWith('5');
      const statusCode = topReq.status || 200;

      const pName = (topReq.provider || 'gateway').toLowerCase();
      const accName = topReq.account || topReq.connectionId || '--';
      const proxyName = topReq.proxy || 'Direct';
      const isRelay = proxyName !== 'Direct' && proxyName !== '--';

      const tr = document.createElement('tr');
      tr.className = 'recent-entry-highlight';
      tr.innerHTML = `
        <td>
          <div style="font-family:var(--mono); line-height:1.2;">
            <span style="font-size:10px; color:var(--text-bright); font-weight:500;">${escapeHtml(timeStr)}</span>
            <small style="display:block; font-size:8px; color:var(--muted);">${escapeHtml(dateStr)}</small>
          </div>
        </td>
        <td><code class="model-id-code" style="font-size:10.5px;">${escapeHtml(topReq.model || '--')}</code></td>
        <td>
          <div style="line-height:1.25;">
            <strong style="color:var(--text-bright); font-size:11px;">${escapeHtml(pName)}</strong>
            <small style="display:block; font-size:8.5px; font-family:var(--mono); color:var(--muted); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; max-width:180px;">${escapeHtml(accName)}</small>
          </div>
        </td>
        <td>
          <div style="line-height:1.25;">
            <span class="table-badge ${isRelay ? 'purple' : ''}" style="font-size:8px; padding:1px 5px;">${escapeHtml(proxyName)}</span>
            ${topReq.strategy ? `<small style="display:block; font-size:7.5px; font-family:var(--mono); color:#71717a; text-transform:uppercase; margin-top:2px;">${escapeHtml(topReq.strategy)}</small>` : ''}
          </div>
        </td>
        <td><span class="table-cell-mono" style="font-size:10px; color:var(--text);">${toks > 0 ? `${toks.toLocaleString()}t` : '--'}</span></td>
        <td style="text-align: right;"><span class="table-badge ${isErr ? 'inactive' : 'active'}" style="font-size:7.5px;">${escapeHtml(String(statusCode))}</span></td>
      `;

      tbody.insertBefore(tr, tbody.firstChild);
      while (tbody.children.length > usageRecentPageSize) {
        tbody.removeChild(tbody.lastChild);
      }
    }

    // 3. Live prepend to Overview Event Activity Box
    const overviewStreamBox = document.querySelector('#overview-recent-stream-box');
    if (overviewStreamBox && topReq && topReq.model) {
      const timeStr = topReq.timestamp ? new Date(topReq.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '--:--:--';
      const isErr = topReq.status === 'error' || String(topReq.status).startsWith('4') || String(topReq.status).startsWith('5');
      const statusCode = topReq.status || 200;
      const statusColor = isErr ? '#ef4444' : '#22c55e';
      const toks = (topReq.promptTokens || 0) + (topReq.completionTokens || 0);
      const pName = (topReq.provider || 'gateway').toLowerCase();
      const cleanP = pName.startsWith('openai-compatible') ? 'custom' : pName;

      const itemEl = document.createElement('div');
      itemEl.className = 'console-log-row';
      itemEl.style.cssText = 'background:#05070a; border:1px solid rgba(255,255,255,0.05); border-radius:6px; padding:6px 10px; display:flex; justify-content:space-between; align-items:center; gap:8px;';
      itemEl.innerHTML = `
        <div style="display:flex; align-items:center; gap:6px; min-width:0; overflow:hidden;">
          <span style="color:${statusColor}; font-weight:bold; font-size:10px; font-family:var(--mono);">● ${escapeHtml(String(statusCode))}</span>
          <span class="method-tag post" style="margin:0; font-size:8px;">POST</span>
          <strong style="color:var(--text-bright); font-size:11px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${escapeHtml(topReq.model || 'model')}</strong>
        </div>
        <div style="display:flex; align-items:center; gap:6px; flex-shrink:0; font-family:var(--mono); font-size:9.5px;">
          <span class="prov-pill ${escapeHtml(cleanP)}" style="font-size:8.5px; padding:1px 4px;">${escapeHtml(cleanP)}</span>
          <span style="color:var(--muted);">${toks > 0 ? `${toks}t` : '--'}</span>
          <span style="color:#52525b; font-size:8.5px;">${escapeHtml(timeStr)}</span>
        </div>
      `;
      overviewStreamBox.insertBefore(itemEl, overviewStreamBox.firstChild);
      while (overviewStreamBox.children.length > 7) {
        overviewStreamBox.removeChild(overviewStreamBox.lastChild);
      }
    }
  }
});

async function bootstrapDashboardAuth() {
  try {
    const response = await fetch(`${apiBase}/api/auth/status`, { credentials: 'same-origin' });
    const status = await response.json().catch(() => ({}));
    dashboardAuthenticated = response.ok && status.authenticated === true;
  } catch {
    dashboardAuthenticated = false;
  }

  if (dashboardAuthenticated) {
    document.querySelector('#full-login-overlay')?.remove();
    setView(window.location.hash.slice(1) || 'overview');
  } else {
    renderFullLoginGate();
  }
}

const initialView = window.location.hash.slice(1);
initMeshZoomPanControls();
bootstrapDashboardAuth();
window.addEventListener('load', () => {
  initMeshZoomPanControls();
  if (!window.location.hash || window.location.hash === '#overview') {
    loadOverview();
  }
});

// Provider CRUD has no dedicated SSE channel yet; poll the lightweight provider
// catalog so changes made from another view or client reach the mesh promptly.
function startMeshProviderSync() {
  if (meshProviderSyncTimer) return;
  meshProviderSyncTimer = window.setInterval(() => {
    const currentView = window.location.hash.slice(1).split('/')[0] || 'overview';
    if (currentView === 'overview' && hasDashboardAccess()) loadOverview();
  }, 2500);
}

startMeshProviderSync();

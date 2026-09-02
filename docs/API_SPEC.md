# Zyrouter REST & Proxy API Specification

Spesifikasi lengkap seluruh endpoint API yang disediakan oleh **Zyrouter Go Engine**.

---

## 1. AI Proxy & Chat Endpoints

### 1.1. `POST /v1/chat/completions` / `POST /chat/completions`
- **Format:** OpenAI Compatible
- **Headers:** `Authorization: Bearer <API_KEY>`
- **Payload:** Standard OpenAI Chat Payload (`model`, `messages`, `temperature`, `stream`, etc.)
- **Response:** JSON response atau SSE Stream (`text/event-stream`).

### 1.2. `POST /v1/messages` / `POST /messages`
- **Format:** Anthropic Claude Compatible
- **Headers:** `x-api-key: <API_KEY>` atau `Authorization: Bearer <API_KEY>`, `anthropic-version: 2023-06-01`
- **Payload:** Standard Claude Payload (`model`, `messages`, `system`, `max_tokens`, `stream`, etc.)
- **Response:** JSON response atau SSE Stream format Claude.

### 1.3. `POST /messages/count_tokens` / `POST /v1/messages/count_tokens`
- **Tujuan:** Menghitung estimasi token untuk request payload Claude.

### 1.4. `POST /api/chat`
- **Format:** Ollama Compatible Chat Endpoint.

### 1.5. Model Discovery
- `GET /models` — Daftar semua model aktif, alias, dan combo.
- `GET /v1/models` — Alias OpenAI-compatible untuk daftar model aktif.
- `GET /models/info` — Metadata detail kapabilitas model (vision, tool-calling, pdf, max context).
- `GET /v1/models/info` — Alias OpenAI-compatible metadata model.
- `GET /models/{kind}` — Filter model berdasarkan kategori (`chat`, `embedding`, `media`).

---

## 2. Retired Extended APIs

Media, Headroom, MITM, CLI tools, search, dan scrape endpoints tidak termasuk runtime proxy-first Zyrouter dan tidak lagi dimount oleh Go router.

---

## 3. Admin & Dashboard Management Endpoints

Semua endpoint berikut memerlukan admin session atau API key non-client.

### 3.1. Provider Connections (`/api/providers`)
- `GET /api/providers` — List semua koneksi provider terdaftar.
- `POST /api/providers` — Tambah koneksi provider baru (API key / OAuth).
- `GET /api/providers/{id}` — Detail satu provider connection.
- `PUT /api/providers/{id}` — Update prioritas, credential, atau status aktif.
- `DELETE /api/providers/{id}` — Hapus koneksi provider.
- `POST /admin/health/reset` — Reset cooldown / lock status provider & model.

### 3.2. Combos (`/api/combos`)
- `GET /api/combos` — List semua combo model & strateginya.
- `POST /api/combos` — Buat combo baru (nama, daftar model, strategi: fallback/round-robin/sticky/fusion).
- `PUT /api/combos/{id}` — Edit konfigurasi combo.
- `DELETE /api/combos/{id}` — Hapus combo.

### 3.3. API Keys & Restrictions (`/api/keys`)
- `GET /api/keys` — List semua API Key yang aktif dan terdaftar.
- `POST /api/keys` — Buat API Key baru beserta restriksi (`allowedModels`, `allowedPrefixes`, `allowedProviders`, `rateLimit`).
- `PUT /api/keys/{id}` — Update nama, status aktif/non-aktif, atau ubah restriksi.
- `DELETE /api/keys/{id}` — Hapus / revoke API Key.

### 3.4. Settings & Token Savers (`/api/settings`)
- `GET /api/settings` — Ambil seluruh setting global (RTK, Caveman, Ponytail, Fusion tuning).
- `PUT /api/settings` — Update setting global.

### 3.5. Proxy Pools (`/api/proxy-pools`)
- `GET /api/proxy-pools` — List proxy pool yang terkonfigurasi.
- `POST /api/proxy-pools` — Tambah proxy baru.
- `POST /proxy-pools/cloudflare-deploy` — Deploy proxy ke Cloudflare Worker.
- `POST /proxy-pools/deno-deploy` — Deploy proxy ke Deno Deploy.
- `POST /proxy-pools/vercel-deploy` — Deploy proxy ke Vercel Edge.
- `POST /api/proxy-pools/vercel-deploy/jobs` — Membuat single/bulk Vercel deployment job. Token hanya hidup di memory selama job.
- `GET /api/proxy-pools/vercel-deploy/jobs` — Daftar job deployment aktif/terakhir di memory proses.
- `GET /api/proxy-pools/vercel-deploy/jobs/{id}` — Status progress job.
- `GET /api/proxy-pools/vercel-deploy/jobs/{id}/stream` — SSE progress job.
- `POST /api/proxy-pools/vercel-deploy/jobs/{id}/cancel` — Membatalkan job berjalan.

Bulk job berjalan sequential dengan batas maksimal 50 project. Delay dapat berupa fixed atau random range. Metadata job tidak menyimpan token Vercel; restart proses akan menghentikan job yang masih berjalan.

### 3.6. Client Dashboard API (`/api/client/*`)
Semua endpoint berikut memakai client access token terbitan admin, bukan admin session atau upstream provider key.

- `GET /api/client/profile` — Profil client terautentikasi.
- `GET /api/client/policy` — Policy prefix read-only yang ditetapkan admin.
- `GET /api/client/keys` — Daftar key milik client tanpa full key.
- `POST /api/client/keys` — Generate key baru dengan policy server-side.
- `DELETE /api/client/keys/{id}` — Revoke key milik client.
- `GET /api/client/usage` — Usage agregat milik client.

Admin provisioning endpoints:

- `POST /api/admin/client-policies` — Membuat policy prefix dan quota.
- `POST /api/admin/clients` — Membuat client dan menerima access token satu kali.

Client tidak dapat mengirim `allowedPrefixes`, `allowedProviders`, atau `allowedModels` untuk menimpa policy.

---

## 4. Real-Time Telemetry & SSE Streams

## 4.0. Audit Training Record

Audit JSONL hanya menyimpan field training inti: `request`, `response`, masked `apiKey`, `provider`, `model`, status, dan timestamp. Header, URL upstream, connection ID, timing detail, token metadata, dan payload duplikat tidak dipersist. API key tidak pernah disimpan penuh; request/response masing-masing dibatasi 64 KiB dan diberi suffix `...[truncated]` jika melebihi batas.

### 4.1. `GET /usage/stream` / `GET /api/usage/stream`
- **Protocol:** Server-Sent Events (`text/event-stream`)
- **Events:**
  - `event: ping` — Keep-alive heartbeat (interval 15s).
  - `event: request_start` — Sinyal request masuk (untuk animasi grafis topologi).
  - `event: request_finish` — Metrik selesai: latensi, status, prompt tokens, completion tokens, cost.

### 4.2. `GET /translator/console-logs/stream`
- **Protocol:** Server-Sent Events (`text/event-stream`)
- **Events:**
  - `event: log` — Real-time log buffer (level, message, timestamp, trace context).

### 4.3. `GET /usage/stats` / `GET /api/usage/stats`
- **Format:** JSON
- **Query Params:** `days=7`, `provider=openai`, `model=gpt-4o`
- **Response:** `activeRequests`, `recentRequests`, `totalRequests`, `promptTokens`, `completionTokens`, `totalTokens`, `totalCost`, dan `daily` breakdown dari SQLite `usageHistory`.

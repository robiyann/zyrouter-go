# Zyrouter Client Portal Blueprint

Dokumen ini adalah kontrak fungsi dan navigasi Client Portal. Dokumen ini tidak menentukan warna, layout, typography, component library, animasi, atau visual design. UI baru wajib mengimplementasikan seluruh fungsi dan state di bawah ini.

## 1. Batas Produk

### Domain

- \`client.zyvenox.tech\`: Client Portal.
- \`api.zyvenox.tech/v1\`: inference API menggunakan Gateway API Key.
- \`panel.zyvenox.tech\`: Admin Portal; tidak boleh ditampilkan atau dibuka dari Client Portal.

### Identitas Client

Client Portal memiliki dua mode login:

1. Telegram User.
2. Machine Client.

Mode Telegram User adalah flow utama untuk user biasa. Kedua mode tidak boleh digabungkan dalam satu session atau satu permission boundary.

### Data yang Dilarang Tampil

Client Portal tidak boleh menampilkan:

- Provider internal.
- Raw upstream model.
- Combo member atau target provider/model.
- Provider connection/account.
- API key user lain.
- Telegram ID atau username user lain.
- Admin console logs.
- Prompt, response body, request headers, IP, user-agent, dan stack trace.

## 2. Global Navigation

### Header

Actions:

- \`Logo / Home\`
  - Tujuan: kembali ke halaman Overview.
- \`Support Telegram\`
  - Link: \`https://t.me/robiyan32\`.
  - Tampilan teks: \`@robiyan32\`.
  - Fungsi: membuka kontak support owner di Telegram.
- \`Account menu\`
  - Menampilkan identitas session aktif.
- \`Logout\`
  - Telegram mode: \`POST /api/user/logout\`.
  - Machine mode: hapus token machine dari storage/session, lalu kembali ke Auth.
  - Menutup seluruh SSE connection.

### Main Navigation

Item wajib:

1. \`Overview\`
2. \`API Keys\`
3. \`Allowed Models\`
4. \`Usage\`
5. \`Global Stream\`
6. \`Playground\`
7. \`API Guide\`

Perubahan tab tidak boleh melakukan full page reload dan tidak boleh menjadi satu-satunya cara untuk mengambil data baru.

## 3. Authentication State Machine

### 3.1 Auth Landing

Elements/actions:

- \`Login Telegram\`
  - Memulai \`POST /api/user/verification/start\`.
- \`Machine Client Login\`
  - Membuka form token \`clt_...\`.
- \`Support @robiyan32\`
  - Membuka \`https://t.me/robiyan32\`.

Initial state:

- Tidak ada client session.
- Tidak ada dashboard content.
- Tidak membuka endpoint admin.

### 3.2 Telegram Verification

#### State: \`pending\`

Response start berisi:

- \`challengeId\`.
- \`expiresAt\`.
- \`telegramDeepLink\`.

UI actions:

- \`Copy Challenge Code\`
  - Menyalin \`challengeId\`.
- \`Open @Zyrouter_bot\`
  - Membuka \`telegramDeepLink\`.
  - Fallback: \`https://t.me/Zyrouter_bot?start=<challengeId>\`.
- \`Cancel\`
  - Menghentikan polling lokal.
  - Menghapus challenge dari state UI.
  - Tidak membuat session.

Polling:

- \`GET /api/user/verification/{challengeId}\`.
- Interval boleh 2 detik.
- Polling berhenti ketika status bukan \`pending\` atau challenge expired.
- Polling tidak boleh menerbitkan session.

#### State: \`telegram_verified\`

Telegram bot mengirim confirmation code one-time, contoh:

\`\`\`
ZV-842913
\`\`\`

UI actions:

- \`Submit Confirmation Code\`
  - Mengirim \`POST /api/user/verification/complete\`.
  - Body:

\`\`\`json
{
  "challengeId": "...",
  "confirmationCode": "ZV-842913"
}
\`\`\`

- \`Open Bot Again\`
  - Membuka deep link challenge yang sama.

Validation error:

- Kode salah: tetap di state confirmation, tampilkan error inline.
- Kode expired: tampilkan action \`Create New Challenge\`.
- Browser binding tidak cocok: minta membuat challenge baru.
- Challenge sudah digunakan: minta login ulang.

#### State: \`completed\`

Jika \`POST /api/user/verification/complete\` berhasil:

- Backend mengikat Telegram ID ke user.
- Backend membuat HttpOnly \`user_session\`.
- Frontend menjalankan \`window.location.replace('/#dashboard')\`.
- Frontend memanggil \`GET /api/user/profile\`.
- Frontend membuka Overview.
- Frontend memulai global telemetry stream.

### 3.3 Machine Client Login

Elements/actions:

- \`Input Client Token\`
  - Format expected: \`clt_...\`.
- \`Connect Machine Client\`
  - \`GET /api/client/profile\` dengan Bearer token.
- \`Clear Token\`
  - Menghapus token lokal dan kembali ke Auth.

Successful login:

- Load \`/api/client/profile\`.
- Load \`/api/client/policy\`.
- Load \`/api/client/keys\`.
- Load \`/api/client/usage\`.
- Load \`/api/client/logs\`.
- Start \`/api/client/global-usage/stream\`.
- Start \`/api/client/logs/stream\` jika private client logs diaktifkan.

## 4. Overview

### Data Pribadi

Load:

- Telegram: \`/api/user/profile\`, \`/api/user/usage\`.
- Machine: \`/api/client/profile\`, \`/api/client/policy\`, \`/api/client/usage\`.

Information:

- Identitas session sendiri.
- Account type/tier atau machine policy.
- Total request pribadi.
- Total prompt tokens pribadi.
- Total completion tokens pribadi.
- Total tokens pribadi.
- Total cost pribadi jika tersedia.
- Jumlah alias yang diizinkan.
- Status API key aktif.

### Data Global

Load dan update:

- Telegram: \`GET /api/user/global-usage\`.
- Machine: \`GET /api/client/global-usage\`.
- Realtime stream:
  - Telegram: \`/api/user/global-usage/stream\`.
  - Machine: \`/api/client/global-usage/stream\`.

Information yang boleh tampil:

- Global total requests.
- Global prompt tokens.
- Global completion tokens.
- Global total tokens.
- Active request count.
- Recent sanitized events.

Global event tidak boleh memiliki identitas user atau provider internal.

## 5. API Keys

### 5.1 Telegram User API Key

Load:

- \`GET /api/user/key\`.

Actions:

- \`Generate API Key\`
  - \`POST /api/user/key\`.
  - Hanya boleh berhasil jika tidak ada active key.
  - Response secret ditampilkan sekali.
- \`Copy Secret\`
  - Hanya tersedia pada one-time reveal state.
- \`Rotate API Key\`
  - \`POST /api/user/key/rotate\`.
  - Membatalkan key lama secara atomik.
  - Menampilkan secret baru satu kali.
- \`Revoke API Key\`
  - \`DELETE /api/user/key\`.
  - Memerlukan confirmation modal/state.
- \`Copy Masked Prefix\`
  - Menyalin prefix tampilan, bukan secret.

Rules:

- Satu verified Telegram user maksimal satu active key.
- Secret lama tidak dapat di-reveal ulang.
- Secret tidak masuk URL, localStorage, log, atau telemetry.

### 5.2 Machine Client Keys

Load:

- \`GET /api/client/keys\`.

Actions:

- \`Create Key\`
  - \`POST /api/client/keys\`.
- \`Copy One-time Secret\`
  - Hanya saat response create diterima.
- \`Revoke Key\`
  - \`DELETE /api/client/keys/{id}\`.

Machine client mengikuti policy admin dan boleh memiliki lebih dari satu key jika policy mengizinkannya.

## 6. Allowed Models

Source:

- Telegram: \`allowedAliases\` dari \`/api/user/profile\`.
- Machine: \`allowedModels\` dari \`/api/client/policy\`.

Per alias tampilkan:

- Public alias.
- Context window jika tersedia.
- Max output tokens jika tersedia.
- Capability flags jika tersedia.
- Status enabled/disabled.

Actions:

- \`Copy Alias\`
  - Menyalin bare alias.
- \`Use in Playground\`
  - Membuka Playground dengan alias terpilih.
- \`Copy cURL Example\`
  - Menghasilkan request dengan alias publik.

Dilarang:

- Tombol fetch provider.
- Provider prefix.
- Raw upstream model.
- Combo target editing.

## 7. Usage Pribadi

Telegram:

- Aggregate: \`GET /api/user/usage\`.
- History: \`GET /api/user/logs?limit=<n>&offset=<n>\`.

Machine:

- Aggregate: \`GET /api/client/usage\`.
- History: \`GET /api/client/logs?limit=<n>&offset=<n>\`.

Table fields:

- Timestamp WIB.
- Request ID.
- Public model alias.
- Status.
- HTTP status.
- Duration.
- Prompt tokens.
- Completion tokens.
- Total tokens.

Actions:

- \`Next Page\`.
- \`Previous Page\`.
- \`Refresh\`.
- Filter by alias/status/date jika backend mendukung.

Private history hanya boleh berisi request milik identity yang sedang login.

## 8. Global Stream

Global stream adalah telemetry gateway yang sudah disanitasi, bukan admin console.

Telegram:

- \`GET /api/user/global-usage/stream\`.

Machine:

- \`GET /api/client/global-usage/stream\` dengan Bearer token.

UI states:

- \`CONNECTING\`.
- \`LIVE\`.
- \`RECONNECTING\`.
- \`PAUSED\`.
- \`OFFLINE\`.

Event fields:

- Timestamp WIB.
- Public model alias.
- Status.
- Token aggregate.
- Duration.

Actions:

- \`Pause Stream\`.
- \`Resume Stream\`.
- \`Clear View\`.
- \`Reconnect\`.

\`Clear View\` hanya membersihkan tampilan browser, tidak menghapus data server.

## 9. Private Client Logs

Jika fitur private logs ditampilkan:

- Telegram: \`/api/user/logs/stream\`.
- Machine: \`/api/client/logs/stream\`.

Private logs hanya berisi request identity yang sedang login. Global Stream dan Private Logs harus diberi label berbeda agar user tidak mengira keduanya sama.

## 10. Playground

Controls:

- \`Model Alias Select\`.
- \`API Key Input\`.
- \`Prompt Input\`.
- \`Send\`.
- \`Stop\`.
- \`Clear Conversation\`.
- \`Copy Request Example\`.

Request target:

\`\`\`
https://api.zyvenox.tech/v1/chat/completions
\`\`\`

Rules:

- Model harus bare public alias.
- Provider prefix harus ditolak sebelum request dikirim.
- API key tidak disimpan permanen.
- Stream response ditampilkan bertahap.
- Stop membatalkan AbortController/fetch.
- Error HTTP ditampilkan tanpa raw provider payload.

## 11. API Guide

Sections:

- Base URL.
- Authentication header.
- Allowed aliases.
- cURL example.
- Python OpenAI example.
- Node.js OpenAI example.
- Streaming example.
- Error codes.
- Key rotation warning.

Semua contoh harus memakai \`https://api.zyvenox.tech/v1\` dan alias publik.

## 12. Error and Session Rules

- \`401\`: session/token expired; close stream and return to Auth.
- \`403\`: permission/policy violation; keep user in current page and show reason aman.
- \`409\`: duplicate active key or state conflict; refresh current resource.
- \`410\`: secret unrecoverable; instruct rotate.
- \`429\`: show rate/quota message and retry guidance.
- \`5xx\`: show gateway unavailable; never show provider payload.
- SSE disconnect: exponential reconnect with visible state.
- No silent redirect loop.

## 13. Responsive and Accessibility Contract

Required viewport checks:

- 375px mobile.
- 768px tablet.
- 1024px laptop.
- 1440px desktop.

Required behavior:

- Every button has a visible label.
- Keyboard focus is visible.
- Form errors appear next to the relevant field.
- Loading state disables only the active action.
- Reduced-motion preference disables non-essential animation.
- No horizontal overflow on mobile.
- Stream list has bounded memory and pagination/history boundary.

## 14. Acceptance Checklist

- [ ] Telegram challenge created once and expires after 10 minutes.
- [ ] Bot deep link opens \`@Zyrouter_bot\`.
- [ ] Bot verifies Telegram ID and returns one-time confirmation code.
- [ ] User enters confirmation code in browser.
- [ ] Backend creates session only after code confirmation.
- [ ] Telegram ID is bound to the correct user.
- [ ] User is redirected to \`#dashboard\` after success.
- [ ] One active Telegram API key invariant works.
- [ ] Allowed models show aliases only.
- [ ] Personal usage is isolated.
- [ ] Global usage totals are visible and sanitized.
- [ ] Global Stream is realtime without tab switching.
- [ ] Private logs, if enabled, are identity-scoped.
- [ ] Playground sends only to \`api.zyvenox.tech/v1\`.
- [ ] Provider prefix/raw model never appears in client controls.
- [ ] Admin UI/routes are inaccessible from Client Portal.
- [ ] \`@robiyan32\` support link works.

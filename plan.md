# Zyrouter Execution Plan

## Status Implementasi — 2026-09-02

- [x] Baseline Go/frontend test dan vet hijau.
- [x] Runtime media, MITM, Headroom, dan CLI tools dipangkas.
- [x] Sisa media handler di package chat dibersihkan dari build graph.
- [x] Deployment Cloudflare/Deno/Vercel dipisahkan dan tetap aktif.
- [x] `/v1` chat/message aliases tersedia.
- [x] Prefix/provider policy enforcement dan fail-closed validation.
- [x] Provider allowlist startup.
- [x] Context preservation regression coverage.
- [x] Native mock-upstream benchmark baseline.
- [x] Runtime smoke test pada port 20128 dan mode Go tanpa frontend.
- [x] Client identity, `clientPolicies`, dan Client API contract implementation (tanpa UI client).
- [x] Client key expiry/quota enforcement dan admin/proxy authorization boundary.
- [x] Native binary deployment path verified; Docker image remains optional (Docker belum tersedia di environment).
- [ ] Race test verification (CGO compiler `gcc` belum tersedia di environment).
- [x] Linux CI workflow disiapkan untuk race, Bash, dan Docker verification.

## Arah Produk

Zyrouter difokuskan sebagai:

- Go Proxy Engine.
- Admin Dashboard.
- Cloudflare, Deno, dan Vercel Edge Proxy Deployment.

Client Dashboard belum dikerjakan sekarang. Namun backend harus menyediakan API contract yang aman agar Client Dashboard dapat dibuat kemudian tanpa mengubah core proxy.

## Scope yang Dipertahankan

- Go proxy engine dan streaming SSE.
- Provider routing, model resolver, provider prefix, fallback, dan round-robin.
- API key authentication, restrictions, health/cooldown, dan usage tracking.
- Admin dashboard untuk provider, combo, key, usage, logs, settings, dan proxy pools.
- Deployment Cloudflare, Deno, dan Vercel.
- Mock upstream dan automated tests.

## Scope yang Dipangkas dari Runtime Utama

- MITM proxy.
- Headroom.
- CLI tools detector.
- Image, audio, video, search, scrape, dan web fetch.
- OAuth provider yang tidak digunakan.
- Executor/provider yang tidak masuk allowlist.

Deployment edge tidak boleh ikut terhapus hanya karena saat ini berada di `handlers/media`; pindahkan deployment ke package terpisah terlebih dahulu.

## Arsitektur Target

```text
Client -> Go Proxy API -> Auth + Prefix Policy -> Resolver -> Router -> Provider Adapter -> Upstream

Admin Dashboard  -> Admin REST API
Future Client UI -> Client REST API
```

Jalur data plane harus ringan. Provider registry, API key lookup, prefix map, dan health state sebaiknya berada di memory. SQLite digunakan untuk persistence, backup, dan usage history.

## Provider Strategy

Gunakan allowlist, misalnya:

```text
ENABLED_PROVIDERS=openai,anthropic,gemini,deepseek,openrouter
```

Minimal pertahankan generic adapter OpenAI-compatible, Anthropic, dan Gemini. Provider-specific executor hanya dipertahankan jika benar-benar digunakan.

## Prefix Policy Foundation

Prefix policy harus disiapkan sekarang walaupun Client Dashboard dibuat nanti.

- Client tidak boleh mengubah `allowedPrefixes` sendiri.
- Policy ditentukan admin/server-side.
- API key mewarisi policy.
- Default policy adalah deny jika prefix tidak diizinkan.
- Validasi dilakukan setelah model, alias, atau combo di-resolve.
- Fallback dan combo tidak boleh menjadi bypass policy.

Endpoint backend yang disiapkan untuk Client Dashboard:

```text
GET  /api/client/profile
GET  /api/client/policy
GET  /api/client/keys
POST /api/client/keys
DELETE /api/client/keys/{id}
GET  /api/client/usage
```

Client API wajib mengambil identitas dari session/token, tidak menerima `clientId` sebagai sumber otorisasi, tidak mengembalikan provider secrets, dan menampilkan full API key hanya sekali.

## Database Target

Tambahkan secara bertahap:

```text
clients
clientPolicies
apiKeys.clientId
apiKeys.policyId
```

API key production idealnya menyimpan hash, prefix tampilan, client ID, policy ID, active state, dan expiration. Full key tidak boleh masuk log.

## Execution Phases

### Phase 1 — Baseline

- Jalankan Go test, vet, build, frontend syntax, dan frontend contract test.
- Catat route/feature matrix terhadap `9router-custom` dan `9router-go-patched`.
- Simpan hasil baseline di `zyrouter/tests/`.

### Phase 2 — Stabilitas

- Fix SQLite test isolation.
- Fix asynchronous log test.
- Fix path test Windows.
- Perbaiki referensi stale `cmd/9router-go` pada Makefile, Dockerfile, dan README.
- Verifikasi atau tambahkan alias `/v1/chat/completions` dan `/v1/messages`.

### Phase 3 — Refactor Runtime

- Pindahkan deployment ke package deployment/proxy pool.
- Hapus media routes dari router utama.
- Hapus atau nonaktifkan MITM, Headroom, dan CLI tools.
- Tambahkan provider allowlist.
- Load konfigurasi provider ke memory.
- Pertahankan proxy pool dan deployment routes.

### Phase 4 — Prefix Governance

- Audit prefix resolver.
- Enforce prefix setelah model resolution.
- Terapkan policy ke direct model, alias, combo, dan fallback.
- Tambahkan model/provider restriction tests.
- Tambahkan schema client policy tanpa membangun UI client.

### Phase 5 — Context Integrity

- Test multi-turn messages dan system prompt.
- Test tools, tool results, reasoning blocks, dan multimodal content.
- Test OpenAI/Anthropic/Gemini round-trip.
- Audit token saver.
- Tambahkan context budget dan compaction terstruktur.

### Phase 6 — Performance

- Reuse HTTP transport dan client.
- Kurangi marshal/unmarshal berulang.
- Stream tanpa buffering response penuh.
- Cache konfigurasi di memory.
- Ukur p50, p95, p99, RPS, memory, dan allocations.

### Phase 7 — Client API Preparation

- Implementasikan client identity.
- Implementasikan policy lookup server-side.
- Implementasikan generate/revoke key.
- Pisahkan usage berdasarkan client.
- Tambahkan contract tests.
- Jangan membuat frontend client pada fase ini.

## Testing Plan

### Static

- `go test ./... -run '^$'`
- `go vet ./...`
- `node --check frontend/app.js`
- `node tests/frontend_contract.test.mjs`

### Unit

- Auth dan restrictions.
- Prefix wildcard.
- Model resolver.
- Provider registry.
- Translator.
- Usage/cost.
- Provider health.
- Proxy pool selection.

### Integration

Gunakan SQLite temporary/in-memory dan `httptest` mock upstream. Jangan menggunakan provider berbayar.

- Authenticated chat request.
- `/v1` aliases.
- OpenAI/Anthropic/Gemini translation.
- Streaming SSE.
- Fallback dan round-robin.
- Provider cooldown.
- Proxy pool routing.
- Provider CRUD dan cascade delete.
- Usage ledger.

### Client API Contract

- Client hanya melihat key miliknya.
- Client tidak dapat mengubah policy atau menambah prefix.
- Prefix terlarang ditolak.
- Combo dan fallback tidak bypass prefix.
- Revoked/expired key ditolak.
- Usage antar client terisolasi.
- Provider secret tidak bocor.

### E2E Flow

```text
client request -> auth -> prefix policy -> resolver -> routing -> mock upstream -> stream -> usage record
```

## Acceptance Criteria

- `go test ./...` PASS.
- `go vet ./...` PASS.
- Frontend syntax dan contract test PASS.
- Docker build optional (native binary deployment adalah target utama).
- Route parity matrix selesai.
- Prefix policy matrix selesai.
- Context preservation matrix selesai.
- E2E streaming dan fallback PASS.
- E2E Client API contract PASS.
- Go proxy dapat berjalan tanpa frontend.
- Deployment Cloudflare/Deno/Vercel tetap berfungsi.

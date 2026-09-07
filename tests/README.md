# Zyrouter Dedicated Testing Sandbox

Folder ini adalah lingkungan terisolasi untuk seluruh pengujian otomatis, benchmarking, dummy test payload, dan simulasi request pada **Zyrouter**.

---

## Aturan Pengujian (Testing Rules)

1. **Jangan Menyimpan File Uji di Luar Folder Ini:**
   - Semua test script (Go `_test.go` integration suites, Node/Python mock scripts, shell curl runners) **WAJIB** berada di dalam `zyrouter/tests/` atau unit test internal Go (`internal/*/*_test.go`).
2. **Isolasi Database Pengujian:**
   - Script test yang memerlukan database harus menggunakan in-memory SQLite (`:memory:`) atau file test temporary di dalam folder ini (misal: `zyrouter/tests/temp_test.db`) dan dihapus setelah test selesai.
3. **Mocking Upstream AI Providers:**
   - Pengujian integrasi tidak boleh melakukan panggilan berbayar ke API upstream nyata kecuali secara eksplisit diminta oleh user. Gunakan mock HTTP test server yang disediakan di dalam suite ini.

---

## Struktur Folder Testing

## Plan Verification Runner

From `zyrouter`:

```powershell
powershell -ExecutionPolicy Bypass -File .\tests\verify_plan.ps1
```

The runner executes:
1. `go test -p 4 ./... -count=1`: Go unit and handler tests across all backend packages.
2. `go vet ./...`: Go static analysis.
3. `go build -trimpath ./cmd/zyrouter`: Native binary compilation.
4. `node --check frontend/app.js`: Frontend JavaScript syntax validation.
5. `node tests/frontend_contract.test.mjs`: REST & SSE API contract assertions on frontend code.
6. `node tests/e2e_proxy_test.mjs`: Automated end-to-end integration test (11 scenarios covering mock upstream, streaming SSE, combo routing, fail-closed prefix rejection, key restrictions, and security summaries).
7. Optional Docker, race, and Bash checks when those tools are available in the environment.

```
zyrouter/tests/
├── README.md                   # Panduan ini
├── verify_plan.ps1             # Runner verifikasi komprehensif
├── frontend_contract.test.mjs  # Verifikasi sinkronisasi kontrak API frontend/backend
└── e2e_proxy_test.mjs          # E2E proxy integration test suite (11 skenario)
```

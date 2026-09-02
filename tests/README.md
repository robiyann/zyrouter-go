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
pwsh -File .\tests\verify_plan.ps1
```

The runner executes Go tests, vet, build, frontend checks, and optional Docker,
race, and Bash checks when those tools are installed.

```
zyrouter/tests/
├── README.md                # Panduan ini
├── integration/             # E2E integration tests (Proxy + Auth Restrictions)
├── benchmarks/              # Throughput, memory allocation & latency benchmark suites
└── mocks/                   # Mock HTTP responses untuk OpenAI, Claude, Gemini
```

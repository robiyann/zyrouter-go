# Zyrouter — System Architecture Specification

> **Active scope update (2026-09-02):** The runtime is proxy-first. Media, MITM, Headroom, and CLI-tool components shown in historical diagrams are retired; edge deployment remains active in `internal/handlers/deployment`.

## Active Runtime Boundary

```text
Client -> Go Proxy -> Auth/Prefix Policy -> Resolver -> Router -> Provider Adapter -> Upstream

Admin Dashboard  -> Admin REST API
Future Client UI -> Client REST API (server-side policy enforced)
Proxy Pool Deployments -> Cloudflare / Deno / Vercel
```

Dokumentasi arsitektur teknis menyeluruh untuk **Zyrouter** (Zyvenox AI Router & Proxy Gateway).

---

## 1. High-Level System Architecture

```mermaid
flowchart TD
    subgraph Clients["Clients & Dev Tools"]
        C1["Claude Code CLI"]
        C2["Codex CLI / Cursor / Qoder"]
        C3["Custom Apps / SDKs"]
        C4["Web Dashboard (Browser)"]
    end

    subgraph ZyrouterEngine["Zyrouter Go Engine (:8080)"]
        direction TB
        MW["Middleware Pipeline\n(RequestID, MaxBody, Logger, Recoverer)"]
        
        Auth["Auth & Key Policy Guard\nValidate APIKey + Model/Prefix/Provider Restrictions"]
        
        subgraph Routers["Domain Handlers"]
            H_Chat["Chat & Messages Handler\n(/v1/chat/completions, /v1/messages)"]
            H_Admin["Admin REST API Handler\n(Providers, Combos, Keys, Settings, Pools)"]
            H_Client["Future Client API Handler\n(Profiles, Client Keys, Client Usage)"]
            H_Stream["SSE Telemetry Handlers\n(/usage/stream, /translator/console-logs/stream)"]
        end

        subgraph CoreLogic["Engine Core & Routing"]
            Resolver["Model Resolver & Combo Engine\n(Fallback / Round-Robin / Sticky / Fusion)"]
            TokenOpt["Token Saver Pipeline\n(RTK / Caveman / Ponytail / Injection Guard)"]
            Translators["Stream & Format Translators\n(OpenAI ↔ Claude ↔ Gemini)"]
            HealthEng["Health Tracker & Exponential Backoff"]
        end

        Repo["SQLite Database Repository\n(WAL Mode, Connection Pool)"]
    end

    subgraph Storage["Persistent Storage"]
        DB[("zyrouter.db (SQLite)")]
    end

    subgraph UpstreamProviders["Upstream AI Providers"]
        P1["OpenAI API"]
        P2["Anthropic Claude API"]
        P3["Google Gemini API"]
        P4["DeepSeek / OpenRouter / Custom"]
        P5["Proxy Pools (Cloudflare/Deno/Vercel)"]
    end

    Clients --> MW
    MW --> Auth
    Auth --> Routers
    Routers --> CoreLogic
    CoreLogic --> Repo
    Repo --> DB
    CoreLogic --> UpstreamProviders
```

---

## 2. Request Lifecycle & Pipeline

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client / Dev Tool
    participant Mid as Middleware & Auth Guard
    participant Res as Model Resolver & Combo
    participant Trans as Format Translator
    participant Prov as Upstream Provider
    participant DB as SQLite DB & SSE Hub

    Client->>Mid: POST /v1/chat/completions (Bearer sk-...)
    Mid->>Mid: Verify API Key & Evaluate Restrictions (Model/Prefix/Provider)
    alt Unauthorized / Restricted
        Mid-->>Client: 401 Unauthorized / 403 Forbidden
    end

    Mid->>Res: Resolve Target Model / Combo Strategy
    Res->>Res: Check Provider Health & Cooldowns
    Res->>Trans: Apply Token Saver & Translate Payload (if needed)
    Trans->>Prov: Forward Request (HTTP / SSE Stream)
    
    alt Streaming Request
        Prov-->>Trans: Stream Chunks (with StallReader timeout guard)
        Trans-->>Client: Format Translation & Stream to Client
    else JSON Request
        Prov-->>Trans: Response JSON
        Trans-->>Client: Format Translation & JSON Response
    end

    Trans->>DB: Asynchronously Log Usage & Emit SSE Event
    DB-->>Client: Real-Time Telemetry Update to Dashboard
```

---

## 3. Komponen Arsitektur Backend (Golang)

### 3.1. `internal/auth`
- **Key Validation:** Verifikasi hash/string API Key terhadap database.
- **Restriction Engine:**
  - `CheckModelAllowed(key, requestedModel)`: Cek apakah model ada di whitelist.
  - `CheckPrefixAllowed(key, requestedModel)`: Cek apakah prefix model sesuai pola (misal: `claude-*`).
  - `CheckProviderAllowed(key, resolvedProvider)`: Cek apakah provider connection diizinkan untuk key ini.

### 3.2. `internal/proxy` & `internal/translator`
- **Zero-Allocation Stream Translators:** Menerjemahkan event SSE antara OpenAI format dan Claude format secara streaming tanpa buffering penuh memori.
- **StallReader Wrapper:** Memonitor setiap chunk upstream dengan timeout 6 menit. Jika koneksi macet, StallReader menutup koneksi dengan aman dan memicu fallback.
- **Multi-Panel Fusion Engine:** Mengirim request paralel ke $N$ model panel, mengumpulkan respons berdasarkan batas toleransi waktu (*straggler grace*), dan mengirim hasil ke model *Judge* untuk sintesis.

### 3.3. `internal/tokensaver`
- **RTK:** Input prompt compression.
- **Caveman & Ponytail:** System prompt injection untuk kontrol gaya respon ringkas/kode efisien.
- **Prompt Injection Guard:** Heuristik deteksi pola jailbreak/injeksi berbahaya.

### 3.4. `internal/db`
- Berbasis `database/sql` + SQLite driver (`modernc.org/sqlite` atau `mattn/go-sqlite3`).
- Mode `WAL` (Write-Ahead Logging) diaktifkan untuk konkurensi baca/tulis tinggi.
- Pragma: `PRAGMA busy_timeout = 5000;`, `PRAGMA journal_mode = WAL;`, `PRAGMA synchronous = NORMAL;`.

---

## 4. Komponen Arsitektur Frontend (100% Redesigned)

### 4.1. Arsitektur State & Data Flow
- **Data Hydration:** Dashboard mengambil konfigurasi awal via REST API Go (`/api/providers`, `/api/combos`, `/api/keys`, `/api/settings`).
- **Live Stream Engine:** Membuka koneksi EventSource ke `/usage/stream` dan `/translator/console-logs/stream` untuk memperbarui topologi lalu lintas dan grafik real-time.
- **Zero Mock Policy:** Komponen UI dirancang dengan empty-state yang elegan saat belum ada provider/kunci yang dikonfigurasi.

### 4.2. Struktur Modul Frontend
```
zyrouter/frontend/
├── src/
│   ├── components/
│   │   ├── ui/             # Button, Modal, Input, Badge, Slider, Switch (Glassmorphism)
│   │   ├── topology/       # Real-time traffic visualizer (SVG/Canvas nodes & edges)
│   │   ├── charts/         # Token usage & cost charts
│   │   ├── forms/          # Provider modal, Combo builder, API Key restriction builder
│   │   └── layout/         # Sidebar, Header, Live status pill, Quick search
│   ├── pages/              # Overview, Providers, Combos, Keys, Usage, Logs, Settings
│   ├── lib/                # API client, SSE managers, formatters, storage helpers
│   └── styles/             # Design tokens, cyber gradients, glow utilities
```

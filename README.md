# Zyrouter Core — High-Throughput AI Gateway & Multi-Model Orchestrator

<div align="center">

[![Go Report Card](https://goreportcard.com/badge/github.com/robiyann/zyrouter-go)](https://goreportcard.com/report/github.com/robiyann/zyrouter-go)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Routing SLO](https://img.shields.io/badge/Routing%20SLO-99.9%25-00FF66?style=flat)](#)
[![Architecture](https://img.shields.io/badge/Architecture-Single%20Binary%20Go-00ADD8?style=flat)](#)
[![Memory Footprint](https://img.shields.io/badge/Memory%20Footprint-~14MB%20RSS-brightgreen?style=flat)](#)

**A high-performance, single-binary AI API gateway and resilient model orchestrator written in Pure Go.**  
*Unified OpenAI/Anthropic API compatibility, dynamic mesh routing, zero-refusal tool cloaking, rotating proxy pools, and enterprise tier-based access governance.*

[Key Features](#-key-features) • [Architecture](#-architecture) • [Dashboard Tour](#-dashboard--ui-walkthrough) • [Quick Start](#-quick-start) • [API Usage](#-api-usage--examples) • [Configuration](#-configuration)

</div>

---

## ⚡ Key Features

* **🚀 Pure Go Single Binary Daemon**:
  * Unified runtime encompassing HTTP API Gateway, PM2 single-process lifecycle, SQLite database engine, and integrated Telegram Quota Bot. Zero external Node.js child daemons or port collisions.
* **🌐 Dynamic Mesh Topology & Realtime Signal Room**:
  * Live interactive network topology visualizer showing incoming clients (Cursor, Claude Code, Cline, OpenCode CLI, Copilot) routed across active upstream account pools.
* **🛡️ Zero-Refusal Quartet Cloaking (PR #4188)**:
  * Native implementation of the OpenCode Free Tier "File-Search Quartet" (`bash`, `glob`, `grep`, `read`) fingerprint injection.
  * Always-stream upstream forwarding paired with high-concurrency in-memory SSE aggregation for non-streaming clients.
  * Native support for outbound rotating proxy relays (Vercel Edge, SOCKS5, HTTP) for IP rate-limit bypass.
* **🔀 Resilient Combo Orchestrator**:
  * Pipeline composability: Compose multi-tier fallback chains, round-robin load distribution, sticky sessions, and model fusion hidden behind single public aliases.
* **👥 Tier-Based User Governance & Quotas**:
  * Granular access tiers (*Administrator*, *Paid User*, *User*) with model allowlists, daily/monthly token caps, and RPM rate limits.
  * Token optimization flags: **RTK** (Real-Time Token Compression for large tool payloads), **Caveman Mode** (concise output token compression), and **Ponytail** context optimization.
* **📊 High-Efficiency Observability Ledger**:
  * Optimized SQLite WAL ledger handling large transaction volumes with sub-millisecond indexed rollup queries and a full payload inspection drawer with raw error classification.
* **⚡ Gzip HTTP Compression & Lazy Chunking**:
  * Automatic `Accept-Encoding: gzip` compression middleware (90%+ bandwidth reduction) and chunked pagination for high-volume provider connection clusters.

---

## 🏗️ Architecture

```text
                                  ┌─────────────────────────────────────────┐
                                  │      Client Layer (OpenAI Format)       │
                                  │ Cursor · Claude Code · Cline · OpenCode │
                                  └────────────────────┬────────────────────┘
                                                       │
                                        HTTPS / API Key (Bearer zy_...)
                                                       │
                                                       ▼
                                  ┌─────────────────────────────────────────┐
                                  │       Cloudflare Edge WAF / Worker      │
                                  │ Rate Limiting & Verification Gateway    │
                                  └────────────────────┬────────────────────┘
                                                       │
                                                       ▼
┌───────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Zyrouter Core Engine (Go Single Daemon — Port 20128)                                                      │
│                                                                                                           │
│  ┌───────────────────────┐   ┌───────────────────────────┐   ┌─────────────────────────────────────────┐  │
│  │   Auth & Middleware   │   │     TokenSaver Pipeline   │   │           Combo Orchestrator            │  │
│  │  - Hashed API Keys    │──►│  - RTK Tool Compression   │──►│  - Round-Robin Load Balancer            │  │
│  │  - Tier Policy Check  │   │  - Caveman Conciseness    │   │  - Fallback / Auto Failover             │  │
│  │  - RPM Rate Limiter   │   │  - Ponytail Simplification│   │  - Sticky Session State                 │  │
│  └───────────────────────┘   └───────────────────────────┘   └────────────────────┬────────────────────┘  │
│                                                                                   │                       │
│  ┌────────────────────────────────────────────────────────────────────────────────▼────────────────────┐  │
│  │ Execution Engines & Translators                                                                     │  │
│  │  - OAuth & Account Connection Pools (Google Cloud, OpenAI Codex, Anthropic, etc.)                   │  │
│  │  - Pure-Go OpenCode Engine (PR #4188 Quartet Cloaking + Upstream Stream Aggregator)                │  │
│  │  - Custom OpenAI / Anthropic Compatible Driver (vLLM, Ollama, LM Studio, Private Endpoints)         │  │
│  └────────────────────────────────────────────────┬────────────────────────────────────────────────────┘  │
│                                                   │                                                       │
│  ┌────────────────────────────────────────────────▼────────────────────────────────────────────────────┐  │
│  │ SQLite Database Layer (WAL Mode)                                                                    │  │
│  │  - Transaction Traces · Indexed Token Rollup · Connection Pools · Auth & Security Event Log         │  │
│  └─────────────────────────────────────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────┬───────────────────────────────────────────────────────┘
                                                    │
                                     Rotating Outbound Proxy Relays
                                                    │
                   ┌────────────────────────────────┼────────────────────────────────┐
                   ▼                                ▼                                ▼
       ┌──────────────────────┐         ┌──────────────────────┐         ┌──────────────────────┐
       │   Google Cloud API   │         │   OpenCode Cloud     │         │   Custom AI / vLLM   │
       │ (Gemini 2.0 / 3.x)   │         │ (Free Tier Models)   │         │ (OpenAI Compatible)  │
       └──────────────────────┘         └──────────────────────┘         └──────────────────────┘
```

---

## 🖥️ Dashboard & UI Walkthrough

### 1. Signal Room & Dynamic Mesh Topology
Live real-time monitoring surface visualizing token throughput, active account topology, and streaming trace activity.

![Overview Topology](docs/screenshots/overview_topology.jpg)

---

### 2. Provider Catalog & Routing Fabric Nodes
Centralized management for OAuth providers, device-flow connections, and custom API clusters.

![Provider Nodes](docs/screenshots/provider_nodes.jpg)

> 📌 **Supported Provider Ecosystem:**
> * **OAuth & Device Flow**: Google Cloud (Gemini), OpenAI Codex, Claude, Cline, CodeBuddy, Trae, Qoder, and Kiro.
> * **Free & Community Tiers**: OpenCode Zen (Pure Go native with PR #4188 tool quartet cloaking and rotating proxy support).
> * **Custom Endpoints**: OpenAI-compatible clusters, Anthropic-compatible servers, vLLM, Ollama, LM Studio, and high-concurrency connection pools.

---

### 3. Combo Orchestrator & Multi-Model Pipelines
Create dynamic fallback pipelines, round-robin load distribution, and multi-model failovers exposed under clean public aliases.

![Combo Orchestrator](docs/screenshots/combo_orchestrator.jpg)

---

### 4. Tier-Based User Governance
Configure account tiers (*Administrator*, *Paid User*, *User*), define daily/monthly token caps, and toggle real-time token optimization flags (*RTK*, *Caveman*, *Ponytail*).

![Account Tiers](docs/screenshots/account_tiers.jpg)

---

### 5. API Key Governance
Create, inspect, and rotate scoped gateway API keys with account type inheritance and model restrictions.

![API Key Governance](docs/screenshots/api_key_governance.jpg)

---

### 6. Usage Ledger & Cost Rollup
Inspect real-time token volume, request traces, estimated provider cost arbitrage, and drill into individual request payload traces.

![Usage Ledger](docs/screenshots/usage_ledger.jpg)

---

## 🚀 Quick Start

### Prerequisites

* Linux / macOS (tested on Ubuntu 22.04+ / Debian 12+)
* **Go 1.22+**
* **SQLite 3**
* **Make** & **PM2** (optional, for background service management)

### 1. Clone & Build

```bash
git clone https://github.com/robiyann/zyrouter-go.git
cd zyrouter-go

# Build single binary with root Makefile
make build
```

The compiled binary will be placed at `backend/zyrouter`.

### 2. Run Tests

```bash
make test
```

### 3. Run with PM2 (Single-Process Daemon)

```bash
pm2 start ecosystem.config.cjs
pm2 save
```

Or run directly:

```bash
./backend/zyrouter
```

---

## 📡 API Usage & Examples

Zyrouter provides a 100% standard OpenAI-compatible endpoint at `/v1/chat/completions`.

### Example cURL (Non-Streaming)

```bash
curl -X POST http://127.0.0.1:20128/v1/chat/completions \
  -H "Authorization: Bearer zy_your_api_key_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model-alias",
    "messages": [
      {"role": "user", "content": "Hello! How does zero-copy work in Linux?"}
    ],
    "stream": false
  }'
```

---

### Example Python (AsyncOpenAI Streaming)

```python
import asyncio
from openai import AsyncOpenAI

client = AsyncOpenAI(
    base_url="http://127.0.0.1:20128/v1",
    api_key="zy_your_api_key_here",
    timeout=60.0
)

async def main():
    stream = await client.chat.completions.create(
        model="your-model-alias",
        messages=[
            {"role": "user", "content": "Explain asynchronous I/O in Go."}
        ],
        stream=True
    )
    async for chunk in stream:
        content = chunk.choices[0].delta.content or ""
        print(content, end="", flush=True)
    print()

if __name__ == "__main__":
    asyncio.run(main())
```

---

## ⚙️ Configuration

Environment variables can be supplied via `.env` or systemd / PM2:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `HOST` | Bind host address | `127.0.0.1` |
| `PORT` | HTTP Server port | `20128` |
| `DB_PATH` | Path to SQLite database | `./data/zyrouter.sqlite` |
| `FRONTEND_DIR` | Directory holding static dashboard assets | `./frontend` |
| `CF_EDGE_SHARED_SECRET` | Shared secret header for Cloudflare Edge Worker | `""` |

---

## 📄 License

Distributed under the [MIT License](LICENSE). Built with ❤️ by [@robiyann](https://github.com/robiyann).

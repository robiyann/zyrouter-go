# Zyrouter Verification Edge Gateway

Worker ini hanya menjadi edge shield untuk endpoint verification. Backend Go
tetap menjadi sumber kebenaran untuk browser binding, Telegram binding,
confirmation code, account creation, dan session.

## Deploy

Set secret tanpa memasukkannya ke git:

```bash
wrangler secret put EDGE_SHARED_SECRET
wrangler deploy
```

Set `CF_EDGE_SHARED_SECRET` dengan nilai yang sama pada environment backend Go.
`ORIGIN_BASE` harus menunjuk ke origin yang hanya dapat menerima traffic dari
Cloudflare/Nginx. Jangan memakai alamat loopback pada Worker.

Route Worker yang dibutuhkan:

```text
client.zyvenox.tech/api/user/verification/*
client.zyvenox.tech/api/telegram/webhook
```

Semua route lain tetap dilayani oleh static client/Nginx. Durable Object dipakai
untuk rate limit atomic per-IP dan per jenis operasi; SQLite Go tetap authoritative.

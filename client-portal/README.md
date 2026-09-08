# Zyrouter Client Portal

Standalone static client portal for `client.zyvenox.tech`.

- Control API: same origin in production; `http://localhost:20128` in local development.
- Inference API: `https://api.zyvenox.tech/v1` in production.
- Local development: `npm run dev`.

The portal exposes no admin routes and stores no Telegram user sessions in browser
storage.

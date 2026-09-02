# Zyrouter Dashboard

The dashboard is served by the Go engine from the same origin as its REST and SSE routes.

## Local fixture run

From `zyrouter/backend` (use the same `DB_PATH` for both commands):

```powershell
$env:DB_PATH = [System.IO.Path]::GetFullPath((Join-Path (Get-Location) "..\tests\dashboard_fixture.sqlite"))
go run ./cmd/seed-dashboard
$env:PORT = "20128"
go run ./cmd/zyrouter
```

Open `http://127.0.0.1:20128/`, then use the profile control to enter the API key created by the fixture. This is a development-only API-key bootstrap, not a dashboard user login. The browser stores it only in `localStorage` under `zyrouter.apiKey`; all API calls use an Authorization Bearer header.

When `DB_PATH` is omitted, the 9router-compatible default remains `%APPDATA%\9router\db\data.sqlite`. The fixture path is intentionally isolated under `zyrouter/tests` so sample records never touch the operator database.

Set `FRONTEND_DIR` when the dashboard assets live outside the default `../frontend` path.

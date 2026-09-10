# Repository Guidelines

## Project Structure & Module Organization

zyrouter/ is the active repository. The Go gateway and REST/SSE API live in
backend/; its entry point is backend/cmd/zyrouter/, with implementation
packages under backend/internal/. The static admin dashboard is in
frontend/, and the separate consumer portal is in client-portal/.
Integration tests, fixtures, and verification scripts belong in tests/.
Architecture, API, and database contracts are documented in docs/.

9router-custom/ and 9router-go-patched/ are read-only reference projects;
do not modify them.

## Build, Test, and Development Commands

Run commands from the relevant directory:

    cd backend
    make build                 # Build the Zyrouter binary
    go test ./... -count=1     # Run all Go tests without cache
    go vet ./...               # Run static analysis
    cd ..
    powershell -ExecutionPolicy Bypass -File .\tests\verify_plan.ps1
    node --check .\frontend\app.js
    cd client-portal; npm run check

Use .\start.ps1 from zyrouter/ to run the gateway and dashboard locally
(default 127.0.0.1:20128). Use make dev for a Go development run. The
verification script also runs frontend contract and mock-upstream E2E tests.

## Coding Style & Naming Conventions

Format Go changes with gofmt; use standard Go package naming, exported
identifiers in PascalCase, and unexported identifiers in camelCase.
Frontend JavaScript uses ES modules, semicolons, and two-space indentation;
keep DOM/API helpers in camelCase. Use descriptive snake_case only where
it is part of an external API or database contract. No repository-wide
JavaScript formatter or linter is configured, so preserve surrounding style.

## Testing Guidelines

Go unit tests use the standard testing package and live beside code as
*_test.go. Node tests use .test.mjs in tests/; use mock HTTP upstreams
and isolated SQLite fixtures—never paid provider calls or production data.
Add regression coverage for changes to routing, auth restrictions, SSE, or
API contracts. Run the full tests/verify_plan.ps1 suite before submitting.

## Commit & Pull Request Guidelines

Use short imperative Conventional Commit-style subjects, matching history:
feat(frontend): ..., fix(auth): ..., or chore(dev): .... Keep commits
focused. Pull requests should explain behavior and affected modules, link the
related task or issue, list verification commands and results, and include
screenshots for dashboard changes. Update relevant docs and CHANGELOG.md.

## Security & Configuration

Never commit .env, tokens, API keys, databases, logs, or generated binaries.
Use .env.example for configuration documentation. Preserve API-key hashing,
restriction checks, provider-prefix enforcement, and fail-closed behavior when
modifying authentication or routing.

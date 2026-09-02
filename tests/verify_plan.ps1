$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$backend = Join-Path $root 'backend'
$failures = @()

function Run-Step([string]$Name, [scriptblock]$Action) {
  Write-Host "[RUN] $Name"
  try {
    & $Action
    Write-Host "[PASS] $Name" -ForegroundColor Green
  } catch {
    $script:failures += $Name
    Write-Host "[FAIL] $Name`: $($_.Exception.Message)" -ForegroundColor Red
  }
}

Push-Location $backend
try {
  Run-Step 'go test' { go test ./... -count=1 }
  Run-Step 'go vet' { go vet ./... }
  Run-Step 'go build' { go build -trimpath ./cmd/zyrouter }

  if (Get-Command gcc -ErrorAction SilentlyContinue) {
    Run-Step 'go race test' { $env:CGO_ENABLED = '1'; go test -race ./... }
  } else {
    Write-Host '[SKIP] go race test: gcc unavailable' -ForegroundColor Yellow
  }

  if (Get-Command bash -ErrorAction SilentlyContinue) {
    Run-Step 'benchmark shell syntax' {
      bash -n benchmark/run_benchmark.sh
      bash -n benchmark/run_comparison.sh
    }
  } else {
    Write-Host '[SKIP] benchmark shell syntax: bash unavailable' -ForegroundColor Yellow
  }
} finally {
  Pop-Location
}

Push-Location $root
try {
  Run-Step 'frontend syntax' { node --check frontend/app.js }
  Run-Step 'frontend contract' { node tests/frontend_contract.test.mjs }

  Run-Step 'dockerfile structure' {
    $dockerfile = Get-Content (Join-Path $root 'backend\Dockerfile') -Raw
    foreach ($required in @(
      'COPY backend/go.mod backend/go.sum ./',
      'COPY backend/ ./',
      'COPY frontend/ ./frontend/',
      '-o zyrouter ./cmd/zyrouter/',
      'ENTRYPOINT ["zyrouter"]'
    )) {
      if (-not $dockerfile.Contains($required)) {
        throw "Dockerfile missing expected instruction: $required"
      }
    }
  }

  if (Get-Command docker -ErrorAction SilentlyContinue) {
    Run-Step 'docker build' { docker build -f backend/Dockerfile -t zyrouter-plan-check . }
  } else {
    Write-Host '[SKIP] docker build: docker unavailable' -ForegroundColor Yellow
  }
} finally {
  Pop-Location
}

if ($failures.Count -gt 0) {
  Write-Host "Failed steps: $($failures -join ', ')" -ForegroundColor Red
  exit 1
}
Write-Host 'Available plan verification steps passed.' -ForegroundColor Green

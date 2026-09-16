$ErrorActionPreference = "Stop"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "ไม่พบ Docker CLI กรุณาติดตั้งและเปิด Docker Desktop ก่อน"
}

$composeEnvArgs = @()
if (Test-Path -LiteralPath ".env.local") {
    $composeEnvArgs = @("--env-file", ".env.local")
} elseif (Test-Path -LiteralPath ".env") {
    $composeEnvArgs = @("--env-file", ".env")
}

docker compose @composeEnvArgs up -d --build

$deadline = (Get-Date).AddMinutes(3)
$apiHealthy = $false
while ((Get-Date) -lt $deadline) {
    try {
        $health = Invoke-RestMethod -Uri "http://localhost:8080/health" -TimeoutSec 5
        if ($health.status -eq "ok") {
            $apiHealthy = $true
            break
        }
    } catch {
        # API is still starting; retry until the deadline.
    }
    Start-Sleep -Seconds 3
}

if (-not $apiHealthy) {
    docker compose @composeEnvArgs ps
    throw "API ยังไม่ healthy ภายใน 3 นาที"
}

$migrationCount = (& docker compose @composeEnvArgs exec -T mysql mysql -udocflow -pdocflow docflow -N -B -e "SELECT COUNT(*) FROM schema_migrations;").Trim()
if ([int]$migrationCount -lt 3) {
    throw "ไม่พบ migration ครบตามที่คาดไว้: $migrationCount"
}

$web = Invoke-WebRequest -Uri "http://localhost:3000" -TimeoutSec 10
if ([int]$web.StatusCode -ne 200) {
    throw "Frontend ตอบกลับสถานะ $($web.StatusCode)"
}

$webApi = Invoke-RestMethod -Uri "http://localhost:3000/backend/health" -TimeoutSec 10
if ($webApi.status -ne "ok") {
    throw "Frontend proxy ไป Backend ไม่สำเร็จ"
}

Write-Output "STACK_OK"
Write-Output "API_HEALTH=ok"
Write-Output "MIGRATIONS=$migrationCount"
Write-Output "FRONTEND_STATUS=$($web.StatusCode)"
Write-Output "FRONTEND_BACKEND_PROXY=ok"

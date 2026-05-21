param(
  [string]$Addr = $(if ($env:ADDR) { $env:ADDR } else { "120.79.8.174:30910" }),
  [int]$Clients = $(if ($env:CLIENTS) { [int]$env:CLIENTS } else { 300 }),
  [int]$SpawnRate = $(if ($env:SPAWN_RATE) { [int]$env:SPAWN_RATE } else { 30 }),
  [double]$OpsPerClient = $(if ($env:OPS_PER_CLIENT) { [double]$env:OPS_PER_CLIENT } else { 5 }),
  [string]$Duration = $(if ($env:DURATION) { $env:DURATION } else { "8m" }),
  [string]$Actions = $(if ($env:ACTIONS) { $env:ACTIONS } else { "move,move,move,attack,boss,heal,switch" })
)

$ErrorActionPreference = "Stop"
$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RootDir

Write-Host "压测目标: $Addr"
Write-Host "压测参数: clients=$Clients spawn_rate=$SpawnRate/s ops_per_client=$OpsPerClient duration=$Duration"
Write-Host "示例: .\scripts\load-test-hpa.ps1 -Addr <公网IP>:30910 -Clients 600 -OpsPerClient 8 -Duration 10m"
Write-Host "观察 HPA 请在另一个终端运行: watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'"
Write-Host ""

$env:GOCACHE = if ($env:GOCACHE) { $env:GOCACHE } else { Join-Path $RootDir ".gocache" }

go run ./cmd/gateway-loadtest `
  -addr $Addr `
  -clients $Clients `
  -spawn-rate $SpawnRate `
  -ops-per-client $OpsPerClient `
  -duration $Duration `
  -actions $Actions

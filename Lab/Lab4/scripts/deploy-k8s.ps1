param(
  [string]$GatewayImage = $(if ($env:GATEWAY_IMAGE) { $env:GATEWAY_IMAGE } else { "lab4/gateway:local" }),
  [string]$CoordinatorImage = $(if ($env:COORDINATOR_IMAGE) { $env:COORDINATOR_IMAGE } else { "lab4/coordinator:local" }),
  [string]$MapImage = $(if ($env:MAP_IMAGE) { $env:MAP_IMAGE } else { "lab4/map:local" }),
  [string]$Namespace = $(if ($env:NAMESPACE) { $env:NAMESPACE } else { "lab4" }),
  [string]$KustomizeDir = $(if ($env:KUSTOMIZE_DIR) { $env:KUSTOMIZE_DIR } else { "" })
)

$ErrorActionPreference = "Stop"
$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $KustomizeDir) {
  $KustomizeDir = Join-Path $RootDir "deploy/k8s"
}

function Invoke-Retry {
  param(
    [int]$Attempts,
    [scriptblock]$Command
  )
  for ($i = 1; $i -le $Attempts; $i++) {
    try {
      & $Command
      return
    } catch {
      if ($i -eq $Attempts) {
        throw
      }
      Write-Warning "Attempt $i failed, retrying in 3 seconds: $($_.Exception.Message)"
      Start-Sleep -Seconds 3
    }
  }
}

Write-Host "==> Apply Kubernetes manifests: $KustomizeDir"
Invoke-Retry 5 { kubectl apply -k $KustomizeDir }

Write-Host "==> Set deployment images"
Invoke-Retry 5 { kubectl -n $Namespace set image deployment/lab4-gateway gateway=$GatewayImage }
Invoke-Retry 5 { kubectl -n $Namespace set image deployment/lab4-coordinator coordinator=$CoordinatorImage }
Invoke-Retry 5 { kubectl -n $Namespace set image deployment/lab4-map-green map=$MapImage }
Invoke-Retry 5 { kubectl -n $Namespace set image deployment/lab4-map-cave map=$MapImage }
Invoke-Retry 5 { kubectl -n $Namespace set image deployment/lab4-map-ruins map=$MapImage }

Write-Host "==> Wait for rollout"
$deployments = @(
  "lab4-gateway",
  "lab4-coordinator",
  "lab4-map-green",
  "lab4-map-cave",
  "lab4-map-ruins"
)
foreach ($deploy in $deployments) {
  Invoke-Retry 5 { kubectl -n $Namespace rollout status "deployment/$deploy" --timeout=240s }
}

Write-Host ""
Write-Host "==> Current status"
kubectl get pods -n $Namespace -o wide
kubectl get svc -n $Namespace
kubectl get hpa -n $Namespace

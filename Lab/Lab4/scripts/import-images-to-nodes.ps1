param(
  [Parameter(Mandatory=$true)]
  [string]$ImageTar,

  [Parameter(Mandatory=$true)]
  [string[]]$Nodes,

  [string]$RemoteUser = $(if ($env:REMOTE_USER) { $env:REMOTE_USER } else { "root" }),
  [string]$RemoteDir = $(if ($env:REMOTE_DIR) { $env:REMOTE_DIR } else { "/root" }),
  [string]$JumpHost = $(if ($env:JUMP_HOST) { $env:JUMP_HOST } else { "" }),
  [string]$JumpUser = $(if ($env:JUMP_USER) { $env:JUMP_USER } else { "" }),
  [ValidateSet("containerd", "docker", "auto")]
  [string]$Runtime = $(if ($env:RUNTIME) { $env:RUNTIME } else { "containerd" })
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $ImageTar)) {
  throw "image tar not found: $ImageTar"
}

$ImageTarPath = Resolve-Path $ImageTar
$RemoteTar = "$RemoteDir/$(Split-Path $ImageTarPath -Leaf)"
if (-not $JumpUser) {
  $JumpUser = $RemoteUser
}

$SshOptions = @("-o", "StrictHostKeyChecking=no")
if ($JumpHost) {
  $SshOptions += @("-o", "ProxyJump=$JumpUser@$JumpHost")
}

foreach ($node in $Nodes) {
  $SshTarget = "${RemoteUser}@${node}"
  $ScpTarget = "${RemoteUser}@${node}:${RemoteTar}"

  Write-Host ""
  Write-Host "==> Upload image tar to $ScpTarget"
  ssh @SshOptions $SshTarget "mkdir -p '$RemoteDir'"
  scp @SshOptions "$ImageTarPath" $ScpTarget

  Write-Host "==> Import images on $node, runtime=$Runtime"
  if ($Runtime -eq "containerd") {
    ssh @SshOptions $SshTarget "ctr -n k8s.io images import '$RemoteTar' && (crictl images 2>/dev/null | grep lab4 || ctr -n k8s.io images ls | grep lab4)"
  } elseif ($Runtime -eq "docker") {
    ssh @SshOptions $SshTarget "docker load -i '$RemoteTar' && docker images | grep lab4"
  } else {
    ssh @SshOptions $SshTarget "if command -v ctr >/dev/null 2>&1; then ctr -n k8s.io images import '$RemoteTar' && (crictl images 2>/dev/null | grep lab4 || ctr -n k8s.io images ls | grep lab4); elif command -v docker >/dev/null 2>&1; then docker load -i '$RemoteTar' && docker images | grep lab4; else echo 'no containerd ctr or docker found' >&2; exit 1; fi"
  }
}

Write-Host ""
Write-Host "All nodes imported the image tar."

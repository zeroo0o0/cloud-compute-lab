param(
  [Parameter(Mandatory = $true)]
  [ValidateSet("autotest", "scoretest")]
  [string]$Mode,
  [string[]]$TestArgs = @()
)

$ErrorActionPreference = "Stop"
$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$Namespace = if ($env:LAB4_NAMESPACE) { $env:LAB4_NAMESPACE } else { "lab4" }
$RemoteDir = if ($env:LAB4_REMOTE_DIR) { $env:LAB4_REMOTE_DIR } else { "/tmp/lab4-test" }
$SshUser = if ($env:LAB4_SSH_USER) { $env:LAB4_SSH_USER } else { "root" }
$SshPort = if ($env:LAB4_SSH_PORT) { $env:LAB4_SSH_PORT } else { "22" }
$GoTarget = if ($Mode -eq "autotest") { "test/autotest.go" } else { "./test/scoretest" }

function Read-Value {
  param(
    [string]$Name,
    [string]$Prompt,
    [string]$DefaultValue = ""
  )
  $current = [Environment]::GetEnvironmentVariable($Name)
  if ($current) { return $current }
  if ($DefaultValue) {
    $value = Read-Host "$Prompt [$DefaultValue]"
    if (-not $value) { $value = $DefaultValue }
  } else {
    $value = Read-Host $Prompt
  }
  if (-not $value) {
    throw "$Name cannot be empty"
  }
  return $value
}

function Test-LocalKubectl {
  if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
    return $false
  }
  kubectl -n $Namespace get svc lab4-gateway *> $null
  return $LASTEXITCODE -eq 0
}

function Invoke-LocalTest {
  $gatewayAddr = Read-Value "LAB4_GATEWAY_ADDR" "Gateway address, for example 120.79.8.174:30910"
  Write-Host "==> Local kubectl is available. Running $Mode locally."
  Push-Location $RootDir
  try {
    $env:LAB4_NAMESPACE = $Namespace
    $env:LAB4_GATEWAY_ADDR = $gatewayAddr
    if (-not $env:LAB4_KUBECTL) { $env:LAB4_KUBECTL = "kubectl" }
    go run $GoTarget @TestArgs
  } finally {
    Pop-Location
  }
}

function Invoke-RemoteTest {
  $sshHost = Read-Value "LAB4_SSH_HOST" "Kubernetes master public IP"
  $script:SshUser = Read-Value "LAB4_SSH_USER" "SSH user" $SshUser
  $script:SshPort = Read-Value "LAB4_SSH_PORT" "SSH port" $SshPort
  $gatewayAddr = Read-Value "LAB4_GATEWAY_ADDR" "Gateway address" "$sshHost`:30910"
  $sshTarget = "$SshUser@$sshHost"

  $archive = Join-Path ([System.IO.Path]::GetTempPath()) ("lab4-test-" + [System.Guid]::NewGuid().ToString("N") + ".tar.gz")
  try {
    Write-Host "==> Pack Lab4 files"
    $oldCopyfile = $env:COPYFILE_DISABLE
    $env:COPYFILE_DISABLE = "1"
    tar --format ustar --exclude ".git" --exclude ".gocache" --exclude ".dist" --exclude "tmp" -czf $archive -C $RootDir .

    Write-Host "==> Copy files to $sshTarget`:$RemoteDir"
    ssh -p $SshPort -o StrictHostKeyChecking=accept-new $sshTarget "rm -rf '$RemoteDir' && mkdir -p '$RemoteDir'"
    scp -P $SshPort -o StrictHostKeyChecking=accept-new $archive "$sshTarget`:/tmp/lab4-test.tar.gz"
    ssh -p $SshPort -o StrictHostKeyChecking=accept-new $sshTarget "tar -xzf /tmp/lab4-test.tar.gz -C '$RemoteDir' && rm -f /tmp/lab4-test.tar.gz"
  } finally {
    $env:COPYFILE_DISABLE = $oldCopyfile
    if (Test-Path $archive) {
      Remove-Item $archive -Force
    }
  }

  Write-Host "==> Run $Mode on remote master"
  $remoteEnv = "LAB4_NAMESPACE='$Namespace' LAB4_GATEWAY_ADDR='$gatewayAddr' LAB4_KUBECTL=kubectl"
  foreach ($name in @("LAB4_SKIP_DISRUPTIVE", "LAB4_SCORE_FAST", "LAB4_SKIP_CHAOS", "LAB4_SCORE_HIGH_CLIENTS")) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ($value) {
      $remoteEnv += " $name='$value'"
    }
  }
  $remoteArgs = ($TestArgs | ForEach-Object { "'$_'" }) -join " "
  $remoteCmd = "cd '$RemoteDir' && { command -v go >/dev/null 2>&1 || { echo 'Go is not installed on the master node.' >&2; exit 127; }; } && { command -v kubectl >/dev/null 2>&1 || { echo 'kubectl is not installed on the master node.' >&2; exit 127; }; } && $remoteEnv go run '$GoTarget' $remoteArgs"
  ssh -p $SshPort -o StrictHostKeyChecking=accept-new $sshTarget $remoteCmd
}

if (Test-LocalKubectl) {
  Invoke-LocalTest
} else {
  Write-Host "==> Local kubectl is not ready for namespace '$Namespace'. Falling back to SSH."
  Invoke-RemoteTest
}

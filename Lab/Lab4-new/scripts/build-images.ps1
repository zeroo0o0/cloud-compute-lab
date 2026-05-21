param(
  [string]$Tag = "local",
  [string]$ImagePrefix = $(if ($env:IMAGE_PREFIX) { $env:IMAGE_PREFIX } else { "lab4" }),
  [string]$RedisImage = $(if ($env:REDIS_IMAGE) { $env:REDIS_IMAGE } else { "redis:7-alpine" }),
  [string]$OutDir = $(if ($env:OUT_DIR) { $env:OUT_DIR } else { ".dist/images" }),
  [string]$GoArch = $(if ($env:GOARCH) { $env:GOARCH } else { "amd64" }),
  [string]$DockerPlatform = $(if ($env:DOCKER_PLATFORM) { $env:DOCKER_PLATFORM } else { "linux/$($env:GOARCH)" }),
  [string]$IncludeRedisImage = $(if ($env:INCLUDE_REDIS_IMAGE) { $env:INCLUDE_REDIS_IMAGE } else { "1" }),
  [switch]$Push
)

$ErrorActionPreference = "Stop"

if ($DockerPlatform -eq "linux/") {
  $DockerPlatform = "linux/$GoArch"
}

$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RootDir

New-Item -ItemType Directory -Force ".dist" | Out-Null
New-Item -ItemType Directory -Force $OutDir | Out-Null

if (-not $env:GOCACHE) {
  $env:GOCACHE = Join-Path $RootDir ".gocache"
}

Write-Host "==> Build linux binaries: GOOS=linux GOARCH=$GoArch"
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = $GoArch

go build -trimpath -ldflags="-s -w" -o ".dist/lab4-cloud-gateway" "./cmd/cloud-gateway"
go build -trimpath -ldflags="-s -w" -o ".dist/lab4-cloud-coordinator" "./cmd/cloud-coordinator"
go build -trimpath -ldflags="-s -w" -o ".dist/lab4-cloud-map" "./cmd/cloud-map"

Write-Host "==> Build docker images: prefix=$ImagePrefix tag=$Tag platform=$DockerPlatform"
docker build --platform $DockerPlatform -f Dockerfile.cloud-gateway -t "$ImagePrefix/gateway:$Tag" .
docker build --platform $DockerPlatform -f Dockerfile.cloud-coordinator -t "$ImagePrefix/coordinator:$Tag" .
docker build --platform $DockerPlatform -f Dockerfile.cloud-map -t "$ImagePrefix/map:$Tag" .

$SaveImages = @(
  "$ImagePrefix/gateway:$Tag",
  "$ImagePrefix/coordinator:$Tag",
  "$ImagePrefix/map:$Tag"
)

if ($IncludeRedisImage -eq "1") {
  Write-Host "==> Pull redis image for offline node import: $RedisImage"
  docker pull --platform $DockerPlatform $RedisImage
  $SaveImages += $RedisImage
}

if ($Push -or $env:PUSH -eq "1") {
  Write-Host "==> Push docker images"
  docker push "$ImagePrefix/gateway:$Tag"
  docker push "$ImagePrefix/coordinator:$Tag"
  docker push "$ImagePrefix/map:$Tag"
}

$TarFile = Join-Path $OutDir "lab4-images-$Tag.tar"
Write-Host "==> Save docker images: $TarFile"
docker save @SaveImages -o $TarFile

Write-Host ""
Write-Host "Images:"
Write-Host "  $ImagePrefix/gateway:$Tag"
Write-Host "  $ImagePrefix/coordinator:$Tag"
Write-Host "  $ImagePrefix/map:$Tag"
if ($IncludeRedisImage -eq "1") {
  Write-Host "  $RedisImage"
}
Write-Host ""
Write-Host "Image tar:"
Write-Host "  $TarFile"

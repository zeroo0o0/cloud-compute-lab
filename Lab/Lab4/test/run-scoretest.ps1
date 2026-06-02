param(
  [string[]]$TestArgs = @()
)

& "$PSScriptRoot\run-test.ps1" -Mode scoretest -TestArgs $TestArgs

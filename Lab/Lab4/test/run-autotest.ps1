param(
  [string[]]$TestArgs = @()
)

& "$PSScriptRoot\run-test.ps1" -Mode autotest -TestArgs $TestArgs

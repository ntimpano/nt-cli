$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# Run with:
# powershell -ExecutionPolicy Bypass -File .\install.ps1

$onWindows = ($IsWindows -eq $true) -or ($env:OS -eq 'Windows_NT')
if (-not $onWindows) {
  Write-Error "This installer only supports Windows."
  exit 1
}

if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
  Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE. Only AMD64 is supported."
  exit 1
}

$openCodeCandidates = @(
  (Join-Path $env:APPDATA 'opencode\opencode.json'),
  (Join-Path $env:USERPROFILE '.config\opencode\opencode.json')
)

$openCodeJsonPath = $openCodeCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $openCodeJsonPath) {
  Write-Error "OpenCode is required. Install from https://opencode.ai"
  exit 1
}

$repo = 'ntimpano/nt-cli'
$version = $env:NT_CLI_VERSION
if ([string]::IsNullOrWhiteSpace($version)) {
  $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
  $version = $release.tag_name
}

$assetName = 'nt-cli_windows_amd64.zip'
$baseUrl = "https://github.com/$repo/releases/download/$version"

$tmpRoot = Join-Path $env:TEMP ("nt-cli-install-" + [System.Guid]::NewGuid().ToString('N'))
$extractDir = Join-Path $tmpRoot 'extract'
$zipPath = Join-Path $tmpRoot $assetName
$checksumPath = Join-Path $tmpRoot 'sha256sums.txt'

try {
  New-Item -ItemType Directory -Path $tmpRoot -Force | Out-Null

  Invoke-WebRequest -Uri "$baseUrl/$assetName" -OutFile $zipPath
  Invoke-WebRequest -Uri "$baseUrl/sha256sums.txt" -OutFile $checksumPath

  $expectedHash = $null
  foreach ($line in Get-Content -Path $checksumPath) {
    if ($line -match '^(?<hash>[A-Fa-f0-9]{64})\s+\*?(?<file>\S+)$') {
      if ($Matches.file -eq $assetName) {
        $expectedHash = $Matches.hash
        break
      }
    }
  }

  if (-not $expectedHash) {
    Write-Error "Could not find checksum entry for $assetName"
    exit 1
  }

  $actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
  if (-not $actualHash.Equals($expectedHash, [System.StringComparison]::OrdinalIgnoreCase)) {
    Write-Error "SHA256 checksum mismatch for $assetName"
    exit 1
  }

  Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

  $installDir = Join-Path $env:LOCALAPPDATA 'nt-cli'
  New-Item -ItemType Directory -Path $installDir -Force | Out-Null

  $exeSource = Join-Path $extractDir 'nt-cli.exe'
  if (-not (Test-Path $exeSource)) {
    Write-Error "nt-cli.exe not found in release archive"
    exit 1
  }
  Copy-Item -Path $exeSource -Destination (Join-Path $installDir 'nt-cli.exe') -Force

  $bundlePath = Join-Path $extractDir '.nt-cli-agents.json'
  if (Test-Path $bundlePath) {
    $timestamp = Get-Date -Format 'yyyyMMddTHHmmssZ'
    Copy-Item -Path $openCodeJsonPath -Destination "$openCodeJsonPath.bak.$timestamp" -Force

    $existing = Get-Content -Path $openCodeJsonPath -Raw | ConvertFrom-Json
    $bundle = Get-Content -Path $bundlePath -Raw | ConvertFrom-Json

    if ($null -eq $existing.agent) {
      $existing | Add-Member -MemberType NoteProperty -Name 'agent' -Value (New-Object PSObject) -Force
    }

    foreach ($prop in $bundle.PSObject.Properties) {
      if ($prop.Name -like 'nt-*') {
        $existing.agent | Add-Member -MemberType NoteProperty -Name $prop.Name -Value $prop.Value -Force
      }
    }

    $jsonTemp = Join-Path $tmpRoot 'opencode.json.tmp'
    $existing | ConvertTo-Json -Depth 20 | Set-Content -Path $jsonTemp -Encoding utf8
    Move-Item -Path $jsonTemp -Destination $openCodeJsonPath -Force
  }

  $normalizedInstallDir = $installDir.TrimEnd('\\')
  $pathHasInstallDir = (($env:PATH -split ';') | ForEach-Object { $_.Trim().TrimEnd('\\') }) -contains $normalizedInstallDir
  if (-not $pathHasInstallDir) {
    Write-Host "Add nt-cli to your user PATH with:"
    Write-Host "[System.Environment]::SetEnvironmentVariable('Path', [System.Environment]::GetEnvironmentVariable('Path','User') + ';$installDir', 'User')"
  }

  Write-Host "nt-cli $version installed successfully at $installDir\nt-cli.exe"
}
finally {
  if (Test-Path $tmpRoot) {
    Remove-Item -Path $tmpRoot -Recurse -Force
  }
}

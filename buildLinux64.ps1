# NetworkFilesTransfer build script

$versionPrefix = "1.0"
$linuxBinaryName = "nft"
$scriptName = "nft.sh"
$payloadMarker = "__NFT_PAYLOAD_BELOW__"

function Get-NextVersion {
param(
[string]$VersionPrefix
)

$versionFile = "version.txt"
$nextPatch = 0

if (Test-Path $versionFile) {
$currentVersion = ((Get-Content $versionFile -Raw) -replace '^\uFEFF', '').Trim()
if ($currentVersion -match '^(?<major>\d+)\.(?<minor>\d+)\.(?<patch>\d+)$') {
$currentPrefix = "$($Matches.major).$($Matches.minor)"
if ($currentPrefix -eq $VersionPrefix) {
$nextPatch = [int]$Matches.patch + 1
}
}
}

return "$VersionPrefix.$nextPatch"
}

function Write-UnixTextFile {
param(
[string]$Path,
[string]$Content
)

$normalized = $Content -replace "`r?`n", "`n"
$encoding = New-Object System.Text.UTF8Encoding($false)
$fullPath = Join-Path (Get-Location) $Path
$directory = Split-Path -Parent $fullPath
if ($directory -and !(Test-Path $directory)) {
New-Item -ItemType Directory -Path $directory -Force | Out-Null
}
[System.IO.File]::WriteAllText($fullPath, $normalized, $encoding)
}

function New-SingleFileRelease {
param(
[string]$TemplatePath,
[string]$PayloadPath,
[string]$OutputPath
)

$encoding = New-Object System.Text.UTF8Encoding($false)
$templateText = ([System.IO.File]::ReadAllText($TemplatePath)) -replace "`r`n", "`n"
$markerLine = "$payloadMarker`n"
$markerCount = ([regex]::Matches($templateText, [regex]::Escape($markerLine))).Count
if ($markerCount -ne 0) {
throw "template $TemplatePath must not contain a '$payloadMarker' line"
}
if (-not $templateText.EndsWith("`n")) {
$templateText += "`n"
}

$payloadBytes = [System.IO.File]::ReadAllBytes($PayloadPath)
$payloadBase64 = [Convert]::ToBase64String($payloadBytes)
$builder = New-Object System.Text.StringBuilder
for ($i = 0; $i -lt $payloadBase64.Length; $i += 76) {
$chunk = $payloadBase64.Substring($i, [Math]::Min(76, $payloadBase64.Length - $i))
[void]$builder.Append($chunk).Append("`n")
}

$outputText = $templateText + $payloadMarker + "`n" + $builder.ToString()
if (-not $outputText.EndsWith("`n")) {
$outputText += "`n"
}
[System.IO.File]::WriteAllText((Join-Path (Get-Location) $OutputPath), $outputText, $encoding)
}

$nextVersion = Get-NextVersion -VersionPrefix $versionPrefix

Write-Host "--- Build Version: $nextVersion ---" -ForegroundColor Cyan
Write-Host "--- Start Compiling Linux (amd64) Binary ---" -ForegroundColor Cyan

$previousGOOS = [Environment]::GetEnvironmentVariable("GOOS", "Process")
$previousGOARCH = [Environment]::GetEnvironmentVariable("GOARCH", "Process")
$payloadRoot = Join-Path $env:TEMP ("nft-payload-" + [Guid]::NewGuid().ToString("N"))
$payloadStaging = Join-Path $payloadRoot "files"
$payloadArchive = Join-Path $payloadRoot "payload.tar.gz"

try {
$env:GOOS = "linux"
$env:GOARCH = "amd64"

go build -ldflags "-X main.appVersion=$nextVersion" -o $linuxBinaryName .

if ($LASTEXITCODE -ne 0) {
Write-Host "Error: Compilation failed!" -ForegroundColor Red
exit $LASTEXITCODE
}

Write-UnixTextFile -Path "version.txt" -Content $nextVersion

Write-Host "--- Packing release files into $scriptName ---" -ForegroundColor Cyan

New-Item -ItemType Directory -Path $payloadStaging -Force | Out-Null

Copy-Item $linuxBinaryName $payloadStaging -Force
Copy-Item "config.json" $payloadStaging -Force
Copy-Item "version.txt" $payloadStaging -Force
Copy-Item "index.html" $payloadStaging -Force
Copy-Item "download.html" $payloadStaging -Force
Copy-Item "app.css" $payloadStaging -Force
Copy-Item "qrcode.min.js" $payloadStaging -Force
Copy-Item "favicon.svg" $payloadStaging -Force
Copy-Item "nft-linux-guide.md" $payloadStaging -Force

Push-Location $payloadStaging
try {
tar -czf $payloadArchive .
if ($LASTEXITCODE -ne 0) {
throw "failed to create payload archive"
}
} finally {
Pop-Location
}

if (Test-Path "dist") {
Remove-Item "dist" -Recurse -Force
}
New-Item -ItemType Directory -Path "dist" | Out-Null

New-SingleFileRelease -TemplatePath $scriptName -PayloadPath $payloadArchive -OutputPath "dist/$scriptName"

Write-Host "--- Release file: dist/$scriptName ---" -ForegroundColor Cyan
} finally {
if (Test-Path $linuxBinaryName) {
try {
Remove-Item $linuxBinaryName -Force -ErrorAction Stop
} catch {
# Ignore cleanup failures for the temporary build artifact.
}
}

if (Test-Path $payloadRoot) {
Remove-Item $payloadRoot -Recurse -Force -ErrorAction SilentlyContinue
}

if ([string]::IsNullOrEmpty($previousGOOS)) {
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
} else {
$env:GOOS = $previousGOOS
}

if ([string]::IsNullOrEmpty($previousGOARCH)) {
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
} else {
$env:GOARCH = $previousGOARCH
}
}

Write-Host "------------------------------------------------" -ForegroundColor Green
Write-Host "Build Complete! Upload only dist/$scriptName to the server." -ForegroundColor Green
Write-Host "------------------------------------------------" -ForegroundColor Green

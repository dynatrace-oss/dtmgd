<#
.SYNOPSIS
    Install dtmgd (Dynatrace Managed CLI) on Windows.
.DESCRIPTION
    Downloads a dtmgd release from GitHub, checks the archive against the
    release's checksums.txt, and installs dtmgd.exe to %LOCALAPPDATA%\dtmgd,
    adding that directory to the user PATH. A failed, corrupted or incomplete
    download leaves an installed dtmgd as it was. checksums.txt comes from the
    same place as the archive, so this does not detect a compromised release
    or mirror: for that, verify the cosign signature (README, "Verifying a
    download").

    Environment variables:
      DTMGD_VERSION         Version to install, e.g. 0.3.1 or v0.3.1 (default: the latest release;
                            required with DTMGD_DOWNLOAD_BASE)
      DTMGD_INSTALL_DIR     Install directory (default: %LOCALAPPDATA%\dtmgd)
      DTMGD_DOWNLOAD_BASE   Mirror of the release downloads, laid out as <base>/v<version>/<asset>
                            (default: https://github.com/dynatrace-oss/dtmgd/releases/download)
      DTMGD_NO_MODIFY_PATH  Set to 1 to leave the user PATH alone
      GITHUB_TOKEN          Sent only to api.github.com, to avoid rate limits when finding the latest release
.EXAMPLE
    irm https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.ps1 | iex
#>

# The script block keeps every setting and variable below out of the caller's
# session: "irm | iex" runs a script in the caller's scope.
& {
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is much slower with a progress bar
# Windows PowerShell 5.1 on older .NET defaults to TLS 1.0/1.1, which GitHub
# refuses. Adds TLS 1.2 to what the process already allows.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$repo = 'dynatrace-oss/dtmgd'

# Errors are thrown, not exit-ed: under "irm | iex", exit would close the
# user's PowerShell session.
function Fail([string]$message) { throw "Error: $message" }

# Resolve an existing path to its canonical long form, avoiding 8.3 short
# names (C:\Users\LONGUS~1\...). Returns the path unchanged if it does not exist.
function Resolve-LongPath([string]$Path) {
    try {
        if (Test-Path $Path) { return (Get-Item $Path).FullName }
        $parent = Split-Path $Path -Parent
        if ($parent -and (Test-Path $parent)) {
            return Join-Path (Get-Item $parent).FullName (Split-Path $Path -Leaf)
        }
    } catch {
        Write-Debug "Resolve-LongPath: could not resolve '$Path': $_"
    }
    return $Path
}

# RuntimeInformation needs .NET Framework 4.7.1+; older Windows PowerShell
# hosts fall back to the environment. PROCESSOR_ARCHITEW6432 is set for a
# 32-bit PowerShell on 64-bit Windows, and names the OS's architecture.
$osArch = $null
try {
    $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
} catch {
    Write-Debug "RuntimeInformation unavailable: $_"
}
if (-not $osArch) {
    $osArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
}
# -in ignores case, so 'Arm64' also matches PROCESSOR_ARCHITECTURE's ARM64.
$arch = switch ($osArch) {
    { $_ -in 'X64', 'AMD64' } { 'amd64' }
    { $_ -in 'Arm64' } { 'arm64' }
    default { Fail "unsupported architecture: $(if ($osArch) { $osArch } else { 'unknown' })" }
}

if ($env:DTMGD_VERSION) {
    $version = $env:DTMGD_VERSION.TrimStart('v')
} elseif ($env:DTMGD_DOWNLOAD_BASE) {
    Fail 'set DTMGD_VERSION with DTMGD_DOWNLOAD_BASE: the latest release is looked up on api.github.com, not on the mirror'
} else {
    Write-Host 'Finding the latest release...'
    $api = "https://api.github.com/repos/$repo/releases/latest"
    $headers = @{}
    if ($env:GITHUB_TOKEN) { $headers['Authorization'] = "Bearer $env:GITHUB_TOKEN" }
    try {
        $release = Invoke-RestMethod -Uri $api -Headers $headers
    } catch {
        Fail "could not query ${api}: $($_.Exception.Message)"
    }
    if (-not $release.tag_name) { Fail 'could not find the latest release' }
    $version = $release.tag_name.TrimStart('v')
}

$base = if ($env:DTMGD_DOWNLOAD_BASE) { $env:DTMGD_DOWNLOAD_BASE.TrimEnd('/') } else { "https://github.com/$repo/releases/download" }
$asset = "dtmgd_${version}_windows_$arch.zip"

$installDir = if ($env:DTMGD_INSTALL_DIR) { $env:DTMGD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'dtmgd' }

$tempDir = Join-Path (Resolve-LongPath ([System.IO.Path]::GetTempPath())) "dtmgd-install-$([System.Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
try {
    Write-Host "Downloading dtmgd $version (windows/$arch)..."
    foreach ($file in @($asset, 'checksums.txt')) {
        try {
            Invoke-WebRequest -Uri "$base/v$version/$file" -OutFile (Join-Path $tempDir $file) -UseBasicParsing
        } catch {
            Fail "could not download $base/v$version/${file}: $($_.Exception.Message)"
        }
    }

    $expected = $null
    foreach ($line in Get-Content (Join-Path $tempDir 'checksums.txt')) {
        $fields = $line.Trim() -split '\s+'
        if ($fields.Count -ge 2 -and $fields[1].TrimStart('*') -eq $asset) { $expected = $fields[0].ToLowerInvariant(); break }
    }
    if (-not $expected) { Fail "checksums.txt has no checksum for $asset" }
    $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $tempDir $asset)).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { Fail "checksum mismatch for ${asset}: expected $expected, got $actual" }

    $extractDir = Join-Path $tempDir 'x'
    try {
        Expand-Archive -Path (Join-Path $tempDir $asset) -DestinationPath $extractDir -Force | Out-Null
    } catch {
        Fail "could not extract dtmgd.exe from ${asset}: $($_.Exception.Message)"
    }
    $exe = Join-Path $extractDir 'dtmgd.exe'
    if (-not (Test-Path $exe)) { Fail "$asset has no dtmgd.exe" }

    # Copy next to the target, then rename, so an installed dtmgd.exe is never
    # left half-written.
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    $installDir = Resolve-LongPath $installDir
    $target = Join-Path $installDir 'dtmgd.exe'
    $staged = "$target.new"
    Copy-Item -Path $exe -Destination $staged -Force
    try {
        Move-Item -Path $staged -Destination $target -Force
    } catch {
        Remove-Item $staged -Force -ErrorAction SilentlyContinue
        Fail "could not replace ${target} (is dtmgd running?): $($_.Exception.Message)"
    }
} finally {
    Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

if ($env:DTMGD_NO_MODIFY_PATH -ne '1') {
    # Compare long forms, so an 8.3 entry already in PATH is not added again.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @($userPath -split ';' | Where-Object { $_ } | ForEach-Object { Resolve-LongPath $_ })
    if ($installDir -notin $entries) {
        $newPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        $env:Path = "$env:Path;$installDir"
        Write-Host "Added $installDir to your user PATH; open a new terminal to use it."
    }
}

Write-Host ''
& $target version
Write-Host "Installed to $target"
Write-Host ''
Write-Host "Next: create a context, store its token, and check it (see https://github.com/$repo#quick-start):"
Write-Host '  dtmgd config set-context <name> --host https://cluster.example.com --env-id <environment-id> --token-ref <name>-token'
Write-Host '  dtmgd config set-credentials <name>-token'
Write-Host '  dtmgd doctor'
}

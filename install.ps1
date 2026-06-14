$ErrorActionPreference = "Stop"

$repo = "delfosti-infra/oraculo-cli"
$installDir = if ($env:ORACULO_INSTALL_DIR) { $env:ORACULO_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\Oraculo" }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { Write-Error "Arquitectura no soportada: $env:PROCESSOR_ARCHITECTURE"; exit 1 }
}
$asset = "oraculo_windows_$arch.exe"

$version = $env:ORACULO_VERSION
if (-not $version) {
    Write-Host "> Buscando la ultima version..."
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "Accept" = "application/vnd.github+json" }
    $version = $rel.tag_name
    if (-not $version) {
        Write-Error "No pude resolver la ultima version. Fija una con `$env:ORACULO_VERSION = 'vX.Y.Z'"
        exit 1
    }
}

$url = "https://github.com/$repo/releases/download/$version/$asset"

Write-Host "> Instalando oraculo $version (windows/$arch)..."
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("oraculo-" + [System.Guid]::NewGuid().ToString() + ".exe")

try {
    Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing
} catch {
    Write-Error "No pude descargar $url . Revisa https://github.com/$repo/releases"
    exit 1
}

try {
    $sumsTmp = Join-Path ([System.IO.Path]::GetTempPath()) ("oraculo-sums-" + [System.Guid]::NewGuid().ToString() + ".txt")
    Invoke-WebRequest -Uri "https://github.com/$repo/releases/download/$version/checksums.txt" -OutFile $sumsTmp -UseBasicParsing
    $line = (Get-Content $sumsTmp | Where-Object { $_ -match ([regex]::Escape($asset) + '\s*$') } | Select-Object -First 1)
    if ($line) {
        $expected = ($line -split '\s+')[0]
        $actual = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLower()
        if ($expected.ToLower() -ne $actual) {
            Remove-Item $tmp -ErrorAction SilentlyContinue
            Write-Error "Checksum no coincide para $asset. Abortando por seguridad."
            exit 1
        }
        Write-Host "OK Checksum verificado"
    }
    Remove-Item $sumsTmp -ErrorAction SilentlyContinue
} catch { }

$dest = Join-Path $installDir "oraculo.exe"
Move-Item -Force $tmp $dest
Write-Host "OK Binario instalado en $dest"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    $newPath = if ([string]::IsNullOrEmpty($userPath)) { $installDir } else { "$userPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "OK Agregue $installDir a tu PATH de usuario. Reinicia la terminal."
} else {
    Write-Host "OK $installDir ya esta en tu PATH"
}

Write-Host ""
Write-Host "Listo. Verifica con:  oraculo --version"

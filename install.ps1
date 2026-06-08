# Instalador de la CLI de Oraculo (DelfosTI) para Windows.
#
#   irm https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.ps1 | iex
#
# Hace `go install`, deja el binario disponible como `oraculo` y agrega el
# directorio de binarios de Go al PATH del usuario. No necesita admin.
$ErrorActionPreference = "Stop"

$module = "github.com/delfosti-infra/oraculo-cli"
$version = if ($env:ORACULO_VERSION) { $env:ORACULO_VERSION } else { "latest" }

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Necesitas Go instalado (https://go.dev/dl/) para instalar la CLI de Oraculo. Si no usas Go, pide el binario precompilado a tu contacto de DelfosTI."
    exit 1
}

Write-Host "> Instalando la CLI de Oraculo ($module@$version)..."
go install "$module@$version"

$gobin = (& go env GOBIN)
if ([string]::IsNullOrEmpty($gobin)) { $gobin = Join-Path (& go env GOPATH) "bin" }

$src = Join-Path $gobin "oraculo-cli.exe"
if (-not (Test-Path $src)) {
    Write-Error "No encontre el binario en $src tras 'go install'."
    exit 1
}

Copy-Item $src (Join-Path $gobin "oraculo.exe") -Force
Write-Host "OK Binario disponible como 'oraculo' en $gobin"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$gobin*") {
    $newPath = if ([string]::IsNullOrEmpty($userPath)) { $gobin } else { "$userPath;$gobin" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "OK Agregue $gobin a tu PATH de usuario. Reinicia la terminal."
} else {
    Write-Host "OK $gobin ya esta en tu PATH"
}

Write-Host ""
Write-Host "Listo. Verifica con:  oraculo --version"

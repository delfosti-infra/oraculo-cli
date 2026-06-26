# Oráculo CLI

CLI oficial de **Oráculo** (DelfosTI). Graba flujos E2E con Playwright (codegen + screenshots + trace) y los sube al backoffice. Oráculo es CLI-first: la CLI ejecuta todo, el backend solo recibe.

## Requisitos

Solo necesitas **Node.js 18+** — la CLI usa `npx playwright` por dentro para grabar y reproducir flows. Los navegadores de Playwright se instalan solos la primera vez que grabas.

> **No necesitas Go.** El instalador baja un binario precompilado para tu sistema.

## Instalación

**macOS / Linux**

```bash
curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.ps1 | iex
```

El instalador deja el comando `oraculo` en tu `PATH` (sin sudo ni admin). Verifica:

```bash
oraculo --version
```

Para actualizar más adelante: `oraculo update`.

## Uso

```bash
oraculo login                        # autentícate contra el backoffice
oraculo init                         # elige tu proyecto → crea oraculo.config.json
oraculo auth                         # captura la sesión autenticada (storageState)
oraculo record checkout --HU=KDP0-6  # graba un flow con Playwright codegen
oraculo push                         # súbelo al backoffice
```

¿Algo no anda? `oraculo doctor` revisa tu entorno (Node, navegadores de Playwright, conexión al backend, sesión y proyecto vinculado). Corre `oraculo --help` para ver todos los comandos.

## Recetas

**Grabar un flow que necesita permisos del navegador**

Si el flujo pide geolocalización (u otro permiso nativo), decláralos al grabar. Se conceden al reproducir el flow para capturar las screenshots y se consolidan en el proyecto (Ajustes › Permisos), para que el worker los aplique en cada ejecución.

```bash
oraculo record mapa --permissions=geolocation --geolocation=-12.0464,-77.0428
```

Permisos soportados: `geolocation`, `notifications`, `camera`, `microphone`, `clipboard-read`, `clipboard-write`. Para `geolocation` pasa las coordenadas con `--geolocation=lat,lng`.

**Instalar una versión específica**

```bash
ORACULO_VERSION=v1.4.0 bash -c "$(curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh)"
```

**Cambiar la carpeta de instalación** (por defecto `~/.local/bin`, en Windows `%LOCALAPPDATA%\Programs\Oraculo`)

```bash
ORACULO_INSTALL_DIR="$HOME/bin" bash -c "$(curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh)"
```

**Compilar desde fuente** (solo desarrollo — requiere Go 1.26+)

```bash
go build -o oraculo .
```

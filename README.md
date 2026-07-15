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
                                     #   sitios con 2FA/OTP: resuelves el doble factor UNA vez
                                     #   en ese navegador; la sesión se reusa hasta expirar
                                     #   (detalle: sección "Sesión autenticada" en /docs del panel)
oraculo record checkout --HU=KDP0-6  # graba un flow con Playwright codegen
oraculo push                         # súbelo al backoffice
```

¿Algo no anda? `oraculo doctor` revisa tu entorno (Node, navegadores de Playwright, conexión al backend, sesión y proyecto vinculado). Corre `oraculo --help` para ver todos los comandos.

## Recetas

**Grabar un flow que necesita permisos del navegador**

No hace falta nada especial. Oráculo **detecta solo** qué permisos del navegador usa el flow (geolocalización, cámara, micrófono, notificaciones, portapapeles) al reproducirlo y los consolida en el proyecto. Para la ubicación resuelve coordenadas aproximadas por IP; si necesitas precisión, ajústalas en **Ajustes › Permisos** del proyecto.

```bash
oraculo record mapa   # los permisos se detectan y se suben solos
```

**Instalar una versión específica**

```bash
ORACULO_VERSION=v1.4.1 bash -c "$(curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh)"
```

**Cambiar la carpeta de instalación** (por defecto `~/.local/bin`, en Windows `%LOCALAPPDATA%\Programs\Oraculo`)

```bash
ORACULO_INSTALL_DIR="$HOME/bin" bash -c "$(curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh)"
```

**Compilar desde fuente** (solo desarrollo — requiere Go 1.26+)

```bash
go build -o oraculo .
```

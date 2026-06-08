# Oráculo CLI

CLI oficial de **Oráculo** (DelfosTI). Graba flujos E2E con Playwright (codegen + screenshots + trace) y los sube al backoffice. Oráculo es CLI-first: la CLI ejecuta todo, el backend solo recibe.

## Requisitos

- **Go 1.21+** — la instalación usa `go install`.
- **Node.js 18+ y npm** — la CLI usa `npx playwright` por dentro.
- **Navegadores de Playwright** — instálalos una vez con `npx playwright install`.

## Instalación

El instalador deja el comando `oraculo` listo en tu `PATH` automáticamente (no necesita sudo ni admin).

**macOS / Linux**

```bash
curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.ps1 | iex
```

Verifica que quedó disponible:

```bash
oraculo --version
```

> ¿Prefieres instalarlo a mano? `go install github.com/delfosti-infra/oraculo-cli@latest` deja el binario como `oraculo-cli` en `$(go env GOPATH)/bin`; tendrás que renombrarlo a `oraculo` y agregar ese directorio a tu `PATH` tú mismo. El instalador de arriba hace ambas cosas por ti.

## Uso

```bash
oraculo login                        # autentícate contra el backoffice
oraculo init                         # elige tu proyecto → crea oraculo.config.json
oraculo auth                         # captura la sesión autenticada (storageState)
oraculo record checkout --HU=KDP0-6  # graba un flow con Playwright codegen
oraculo push                         # súbelo al backoffice
```

Corre `oraculo --help` para ver todos los comandos.

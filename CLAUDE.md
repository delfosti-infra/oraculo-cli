# Oráculo CLI — Claude Code Working Guide

Este archivo configura cómo Claude debe trabajar en este repo. **Es de lectura obligatoria al inicio de cada feature/fix.**

---

## 1. Stack

- **Lenguaje**: Go 1.26+
- **CLI framework**: Cobra (`github.com/spf13/cobra`)
- **UI/output**: lipgloss (`github.com/charmbracelet/lipgloss`)
- **HTTP client**: stdlib `net/http` con timeout custom
- **Módulo**: `github.com/delfosti-infra/oraculo-cli`
- **Layout**: `cmd/` (comandos) + `internal/api/` (cliente HTTP al core) + `internal/ui/` (output)

---

## 2. Rol del CLI en el ecosistema Oráculo

Oráculo es **CLI-first**. El CLI ejecuta TODO (Playwright codegen, screenshots, trace, push de artefactos al core). El backend `oraculo-core` solo recibe.

- `oraculo record <name>` → ejecuta Playwright codegen + captura screenshots + trace + opcionalmente `--hu KDP0-6` para meta sidecar
- `oraculo push` → empuja spec + screenshots + trace + meta al core
- `oraculo login` → autentica contra el core, guarda JWT
- `oraculo init` → bootstrap del proyecto (genera `oraculo.config.json`)
- `oraculo check` → valida configuración
- `oraculo jira-meta` → workflow específico de Jira

**NO** proponer ejecutar Playwright/codegen server-side. El CLI es soberano.

---

## 3. CodeGraph — usar SIEMPRE antes de codear

Si este repo tiene `.codegraph/` indexado, las queries son sub-millisecond. Si no lo tiene, inicializar con `codegraph init -i`.

**Reglas duras:**

- **NUNCA** uses `grep`/`find` para buscar un símbolo por nombre. Usá `codegraph_search`.
- **NUNCA** chainees `codegraph_search + codegraph_node`. Usá `codegraph_context` (un solo round-trip).
- Para entender un comando/módulo desconocido: `codegraph_explore`.
- Antes de refactorizar un símbolo (rename, delete): `codegraph_impact` para ver el blast radius.
- Después de editar un archivo, esperá al próximo turno antes de re-querear (el watcher tiene ~500ms de debounce).

**Mapa de decisión:**

| Pregunta | Tool |
|---|---|
| "¿Dónde está X definido?" | `codegraph_search` |
| "¿Qué llama a Y?" | `codegraph_callers` |
| "¿Qué llama Y?" | `codegraph_callees` |
| "¿Qué rompo si cambio Z?" | `codegraph_impact` |
| "Dame contexto para esta tarea" | `codegraph_context` |
| "Tour por un comando nuevo" | `codegraph_explore` |
| "¿Qué hay en path/?" | `codegraph_files` |

**Excepción única:** búsqueda de texto literal (mensaje de error, regex de Playwright capture, string constante). Ahí sí grep.

---

## 4. Skills — invocar las relevantes en CADA feature/fix

Las skills viven en `.claude/skills/`. **Antes de empezar cualquier feature o fix, identificá las skills relevantes y reléelas.** No son decorativas.

### Stack core (SIEMPRE relevantes)

| Skill | Cuándo usar |
|---|---|
| **clean-architecture-patterns** | Crear comando nuevo, agregar endpoint al cliente HTTP, dudás dónde poner un helper. |
| **clean-code-antipatterns** | Antes de commit (checklist). Al revisar PRs. Cuando dudás si un pattern es válido. |
| **golang-patterns** | Idiomas Go (error wrapping, defer, context, interfaces, generics). |

### Stack secundario

| Skill | Cuándo usar |
|---|---|
| **golang-testing** | Cuando escribas tests (unit o e2e). |

### Skills propias (mandatory read before any work)

- **`clean-architecture-patterns`** — convención de Oráculo CLI. Layout cmd/internal, separación Cobra vs lógica, HTTP client, types, UI.
- **`clean-code-antipatterns`** — qué NO hacer. Errores de error handling, HTTP client, Cobra, side-effects, UI, type safety.

---

## 5. Flujo de trabajo recomendado

Para cada feature nueva o fix:

1. **Planear con CodeGraph**
   - `codegraph_context` con el símbolo/comando afectado
   - `codegraph_impact` si vas a renombrar/borrar algo

2. **Leer las skills relevantes**
   - Mínimo: `clean-architecture-patterns` (estructura) y `clean-code-antipatterns` (qué evitar)
   - Según pieza: `golang-patterns` para idioms Go, `golang-testing` si escribís tests

3. **Implementar respetando convenciones**
   - Un comando = un file en `cmd/`
   - `RunE` solo parsea + delega a `runX(opts XOpts) error`
   - HTTP requests pasan por `internal/api/client.go`
   - Types compartidos en `internal/api/types/X.type.go`
   - Output al usuario vía `internal/ui/` (no `fmt.Println` directo)
   - Errors wrapped con `fmt.Errorf("contexto: %w", err)`

4. **Pre-commit checklist** (de `clean-code-antipatterns` sección J + `clean-architecture-patterns` sección 11)
   - `go build ./...` ok
   - `go vet ./...` limpio
   - No binarios commiteados (`oraculo-windows.exe` debería estar en .gitignore)
   - No `fmt.Println("DEBUG: ...")`
   - HTTP client tiene timeout
   - `defer resp.Body.Close()` después del check de err
   - godoc en exported funcs/types

5. **Si encontrás un antipattern nuevo**: agregarlo a `clean-code-antipatterns`. Si encontrás un caso de arquitectura no cubierto: agregarlo a `clean-architecture-patterns`. Estas skills son vivas.

---

## 6. No-go zones

- **NO** proponer ejecución de Playwright/codegen server-side. CLI ejecuta, core recibe.
- **NO** usar `http.DefaultClient` (sin timeout). Usar `*Client` con timeout explícito.
- **NO** lógica adentro del closure `RunE`. Va en `runX(opts)` separado.
- **NO** mezclar `fmt.Println` con UI. Output va via `internal/ui/`.
- **NO** `panic` en código de runtime. Devolver `error` y propagar.
- **NO** path concatenation con `+ "/"`. Usar `filepath.Join`.
- **NO** type assertion sin OK pattern: `v.(string)` ❌, `v.(string); if !ok { ... }` ✅.
- **NO** commitear binarios (`oraculo-windows.exe`, etc.). Agregar a `.gitignore`.
- **NO** commitear con `Co-Authored-By: Claude` (Eric lo pidió explícito).
- **NO** ignorar errores con `_, _ = ...` salvo casos justificados.
- **NO** reescribir/mover un tag de versión ya publicado (`git tag -f` + `push --force`). Los tags de Go son **inmutables en el proxy**: `proxy.golang.org` y `sum.golang.org` fijan el contenido de cada versión para siempre, así que `go install ...@vX.Y.Z` seguirá trayendo el commit original por más que muevas el tag en GitHub. Para publicar un fix **siempre bumpear** a una versión nueva (`v1.1.4`, `v1.1.5`, …), nunca reciclar la anterior. (Pasó: mover `v1.1.3` no llegó nunca a la máquina del usuario; tuvimos que sacar `v1.1.4`.)

### Releasear una versión nueva
1. Commit + `push origin main`.
2. `git tag -a vX.Y.Z -m "..."` + `git push origin vX.Y.Z` (tag nuevo, jamás `-f`).
3. Instalar exacto para evitar el lag del endpoint `@latest` del proxy: `ORACULO_VERSION=vX.Y.Z bash -c "$(curl -sSL .../install.sh)"` (install.sh respeta `ORACULO_VERSION`).

---

## 7. Convenciones específicas observadas

### Naming
- Comandos: `loginCmd`, `recordCmd`, `pushCmd` (sufijo Cmd, camelCase).
- Flags como vars del package: `var recordHUFlag []string`.
- Helpers privados: `runRecord`, `slugify`, `loadConfig`.
- Types HTTP: `LoginRequest`, `AuthSession`, `ErrorResponse` (PascalCase exported).

### Error wrapping
Patrón estándar:
```go
if err != nil {
    return fmt.Errorf("operación específica: %w", err)
}
```
El `%w` preserva la cadena para `errors.Is` / `errors.Unwrap`.

### HTTP method canónico
```go
resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(body))
if err != nil { return ... }
defer resp.Body.Close()
respBody, err := io.ReadAll(resp.Body)
if err != nil { return ... }
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    // parse error response
}
// unmarshal success
```

### Mensajes al usuario
- Success: `ui.Success("flow grabado")` → stdout con ✓
- Error: vía error return; root imprime a stderr con ✗
- Progress: `ui.Step("Grabando", "login.spec.ts")` → stdout

---

## 8. Otros repos del monorepo Oráculo

- **oraculo-core** (NestJS backend) — recibe los push del CLI. Mismas reglas de Clean Architecture pero TypeScript. Endpoints relevantes: `POST /auth/login`, `POST /projects/:slug/flows` (push), `POST /workflows/:id/executions/:id/screenshots`, `POST /workflows/:id/executions/:id/trace`.
- **oraculo-admin** (Angular 19) — frontend. No consume CLI, comparte el core.
- **oraculo-worker** — worker headless que también pega al core.

Si tocás endpoints en `internal/api/client.go`, asegurate de matchear los DTOs reales del core (revisar `oraculo-core/src/infrastructure/controllers/...`).

Cuando navegues entre repos, **el CLAUDE.md de cada uno manda**. Este aplica solo a `oraculo-cli`.

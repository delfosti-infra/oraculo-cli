---
name: clean-code-antipatterns
description: Antipatrones de clean code observados en oraculo-cli (Go 1.26 + Cobra + lipgloss UI + HTTP client). Catálogo de patterns para NO escribir y cómo detectarlos en review. Usar al escribir comandos, llamadas HTTP al core, lógica de UI, o cualquier cambio en cmd/ o internal/.
metadata:
  type: review-checklist
  scope: oraculo-cli
---

# Clean Code Antipatterns — Oráculo CLI (Go)

Catálogo de antipatrones reales encontrados en oraculo-cli (Go 1.26, Cobra, lipgloss, HTTP client custom). Cada antipatrón tiene trigger + por qué duele + fix concreto.

Usar como checklist cuando:
- Agregás un comando Cobra nuevo
- Tocás `internal/api/client.go` (cliente HTTP al core)
- Tocás `internal/ui/*` (output formateado)
- Antes de cada commit / al revisar PRs

---

## A. Naming & estructura

### A1. Nombres con `_` o redundancia con package
**Trigger:** `api.ApiClient`, `ui.UIBanner`, `cmd_login`.
**Por qué duele:** Go idiom es package name + tipo. `api.Client` ya implica que es de api.
**Fix:** `api.Client` (no `api.ApiClient`), `ui.Banner` (no `ui.UIBanner`).

### A2. Files largos por concentrar varios comandos / responsabilidades
**Trigger:** `cmd/record.go` con 346 LOC mezclando codegen, screenshot capture, meta sidecar y push.
**Por qué duele:** Más difícil de testear, refactorizar y leer.
**Fix:** Dividir por responsabilidad: `cmd/record.go` (comando + flags), helpers extraídos a `cmd/record_codegen.go`, `cmd/record_meta.go` o a un package `internal/playwright/`.

### A3. Variable global mutable como flag holder en cmd/
**Trigger:** `var recordHUFlag []string` global mutado por Cobra y leído desde el RunE handler.
**Por qué duele:** Estado oculto entre comandos. Tests en paralelo se pisan.
**Fix:** Para el primer comando es OK (el patrón de Cobra). Pero NO compartirlo entre comandos. Cada comando declara sus propias flags.

### A4. Exported sin razón
**Trigger:** `func (c *Client) parseError(...)` que solo se usa adentro del package, pero está exportada (`ParseError`).
**Fix:** lowercase first letter para unexported. Solo Pascal cuando otro package lo necesita.

---

## B. Manejo de errores (la zona crítica en Go)

### B1. `err != nil` sin contexto (`return err`)
**Trigger:**
```go
resp, err := http.Get(url)
if err != nil {
    return err
}
```
**Por qué duele:** Cuando el error se propaga hasta la raíz, el usuario ve `connection refused` sin saber QUÉ operación falló.
**Fix:** `fmt.Errorf("login: %w", err)` para envolver con contexto manteniendo unwrap-ability.

### B2. Crear errores con strings inline sin tipo
**Trigger:** `return errors.New("usuario no encontrado")`.
**Por qué duele:** El caller no puede hacer `errors.Is(err, ErrUserNotFound)`. No es distinguible de otros errors de string.
**Fix:** Para errores que el caller debe inspeccionar, declarar sentinel: `var ErrUserNotFound = errors.New("usuario no encontrado")`. Para errores que solo se reportan al usuario, `fmt.Errorf` está bien.

### B3. Panicking en código de aplicación
**Trigger:** `panic(err)` en cualquier función que no sea `main` o init.
**Fix:** Devolver error. `panic` solo para errores programáticos (índice fuera de rango, nil pointer real), no para errores de runtime esperables (HTTP failures, files missing).

### B4. Ignorar error con `_`
**Trigger:** `_, _ = io.ReadAll(resp.Body)`.
**Por qué duele:** Esconde fallas reales. Si readAll falla, no hay payload para parsear.
**Fix:** Tratar el error. `_` solo para casos donde el error genuinamente no importa (cerrar body en defer en función que ya falló).

### B5. `defer resp.Body.Close()` antes de chequear error
**Trigger:**
```go
resp, err := http.Get(url)
defer resp.Body.Close() // ← resp puede ser nil
if err != nil { return err }
```
**Por qué duele:** Si `err != nil`, `resp` puede ser nil → panic en el Close.
**Fix:** Chequear error PRIMERO, defer DESPUÉS.

### B6. Error strings con capitalización / puntuación incorrecta
**Trigger:** `errors.New("No se pudo conectar.")`.
**Fix:** Go style: lowercase, sin punto final. `errors.New("no se pudo conectar al api")`. Pero acá tenemos español en UX → consistencia con resto del repo es más importante que el style guide de Go en este caso particular.

### B7. Multiple checks anidados en lugar de early return
**Trigger:**
```go
if user != nil {
    if user.IsActive {
        if token, err := generate(); err == nil {
            return token, nil
        }
    }
}
return "", errors.New("...")
```
**Fix:** Early return:
```go
if user == nil { return "", ErrNoUser }
if !user.IsActive { return "", ErrInactive }
token, err := generate()
if err != nil { return "", fmt.Errorf("generate token: %w", err) }
return token, nil
```

---

## C. HTTP client

### C1. Sin timeout
**Trigger:** `http.Get(url)` (usa `DefaultClient` que NO tiene timeout).
**Por qué duele:** El CLI queda colgado indefinidamente si el server no responde.
**Fix:** Cliente custom con `Timeout: 30 * time.Second`. Patrón ya en `internal/api/client.go`.

### C2. No leer y cerrar body cuando hay error
**Trigger:** `if resp.StatusCode != 200 { return err }` sin leer body.
**Por qué duele:** Pool de conexiones HTTP no reusa la conexión. Lento bajo carga.
**Fix:** Leer body (al menos `io.Copy(io.Discard, resp.Body)`) antes de cerrar.

### C3. Body no cerrado
**Trigger:** `resp, _ := http.Post(...)` sin `defer resp.Body.Close()`.
**Fix:** Defer inmediatamente después del check de error.

### C4. Reusar `Client` con contexto / timeout inadecuados
**Trigger:** Hardcodeado 30s para todos los endpoints, incluyendo uploads grandes.
**Fix:** Pasar `context.Context` con timeout específico al método: `c.UploadTrace(ctx, ...)`. Endpoints de upload usan timeout más largo.

### C5. JSON marshal/unmarshal sin tipo
**Trigger:** `var data map[string]interface{}; json.Unmarshal(body, &data)`.
**Por qué duele:** Pierde type safety. Acceder a `data["foo"].(string)` falla en runtime si el shape cambia.
**Fix:** Definir struct en `internal/api/types/` y unmarshal a la struct.

### C6. Repetir lógica de request/parse en cada método del client
**Trigger:** `Login`, `PushFlow`, `UploadTrace` cada uno con el mismo bloque de marshal/POST/check status/parse error.
**Fix:** Helper privado `c.do(method, path, body, &result)` que centraliza marshal, request, status check, parse de error. Cada método público es un wrapper de 3-4 líneas.

---

## D. Cobra command patterns

### D1. Lógica de negocio dentro del `RunE` handler
**Trigger:** Comando Cobra con 200 LOC adentro del closure.
**Por qué duele:** No testeable. Mezcla parseo de flags con la operación real.
**Fix:** `RunE` solo parsea flags y llama a una función pura: `runRecord(opts RecordOpts) error`. La función está en el mismo file pero separada del Cobra binding.

### D2. Mensajes al usuario vía `fmt.Println` dentro del comando
**Trigger:** `fmt.Println("✓ Grabación completada")` esparcido por cmd/.
**Por qué duele:** Estilo inconsistente. No respeta `--quiet` o `--no-color`. Difícil de cambiar en bloque.
**Fix:** Centralizar en `internal/ui/feedback.go` con funciones nombradas: `ui.Success(msg)`, `ui.Step(label, value)`. Patrón ya existe.

### D3. Comandos sin `Args:` validator
**Trigger:** `Use: "record <nombre>"` pero sin `Args: cobra.ExactArgs(1)`.
**Por qué duele:** Usuario corre `oraculo record` sin args y el handler panica con index out of range.
**Fix:** Siempre `Args: cobra.ExactArgs(N)` o `cobra.MinimumNArgs(1)`.

### D4. Flags con nombres inconsistentes
**Trigger:** `--HU` con mayúscula cuando el resto son lowercase.
**Por qué duele:** UX inconsistente.
**Fix:** Convención lowercase-kebab: `--hu`, `--include-linked`. Si querés mayúsculas por razón de UX, documentalo y aplicalo consistentemente.

### D5. Sin `--help` examples
**Trigger:** Comando complejo sin `Example:` field.
**Fix:** Cobra acepta `Example: "  oraculo record login --hu KDP0-6\n  oraculo record signup"`. Acelera onboarding.

---

## E. Estado, archivos, side-effects

### E1. Working directory asumido
**Trigger:** `os.WriteFile("oraculo.config.json", ...)`.
**Por qué duele:** Si el usuario corre desde un subdir, los archivos terminan en lugar equivocado.
**Fix:** Resolver paths relativos al config root (con `os.Getwd()` + búsqueda hacia arriba), o tomarlo de un flag `--cwd`.

### E2. Read/Write sin permisos explícitos
**Trigger:** `os.WriteFile(path, data, 0644)`.
**Para tokens/credentials:** `0600` (solo owner). El `0644` es default razonable para archivos generados.

### E3. Crear dir sin `MkdirAll`
**Trigger:** `os.Mkdir(path)` cuando el parent puede no existir.
**Fix:** `os.MkdirAll(path, 0755)` siempre. Idempotente.

### E4. Path concatenado con `+ "/"`
**Trigger:** `dir + "/" + name`.
**Por qué duele:** Falla en Windows. Bug latente porque dev lo prueba en Mac.
**Fix:** `filepath.Join(dir, name)`. Patrón portable.

### E5. Leer/escribir archivos sensibles a la encoding del OS
**Trigger:** Leer `oraculo.config.json` con `os.ReadFile` y asumir UTF-8.
**Por qué duele:** Usuarios Windows con CRLF pueden romper parse.
**Fix:** Aceptar UTF-8 con o sin BOM. Si parseás JSON, `json.Unmarshal` ya lo hace bien. Solo cuidar al editar manualmente.

---

## F. Concurrencia

### F1. Goroutine sin sync mechanism
**Trigger:** Lanzar goroutine que escribe en variable compartida sin mutex/channel.
**Fix:** Usar channels para comunicación, mutex (`sync.Mutex`) si compartis estado, o evitar la concurrencia si no se justifica.

### F2. `go fn()` sin `WaitGroup` para esperar resultado
**Trigger:** `go uploadScreenshot(...)` en loop sin esperar al final.
**Por qué duele:** El programa termina antes de que las goroutines completen.
**Fix:** `sync.WaitGroup` o errgroup (`golang.org/x/sync/errgroup`) para concurrencia con cancelación + error aggregation.

### F3. Goroutine sin cancelación por context
**Trigger:** Long-running goroutine que nunca lee `ctx.Done()`.
**Fix:** Toda goroutine que puede tardar acepta `ctx context.Context` y usa `select { case <-ctx.Done(): return }`.

---

## G. UI / output al usuario

### G1. ANSI codes inline en strings
**Trigger:** `fmt.Println("\033[32mOK\033[0m")`.
**Fix:** Usar `lipgloss` o el package `internal/ui/` ya armado. Centralizado.

### G2. Output mezclando stdout y stderr arbitrariamente
**Trigger:** Errores en `fmt.Println` (stdout) cuando deberían ir a stderr.
**Fix:** Mensajes de error → `fmt.Fprintln(os.Stderr, ...)`. Output de comando → stdout. Permite `oraculo cmd > file.txt` sin contaminar el archivo con mensajes de progreso.

### G3. Hardcoded "✓" / "✗" / colores que rompen en Windows cmd
**Fix:** Usar `lipgloss` que detecta capabilities y degrada. O al menos verificar `term.IsTerminal()` antes de inyectar colors.

### G4. Mensajes inconsistentes entre comandos
**Trigger:** Un comando usa "✓ OK", otro usa "→ Done", otro usa "Listo!".
**Fix:** Vocabulario común en `internal/ui/feedback.go`. Todos los comandos usan los mismos prefixes/símbolos.

### G5. Imprimir el body crudo del API al usuario en un error
**Trigger:** `fmt.Errorf("status %d: %s", code, string(respBody))` → el usuario ve el envelope JSON entero: `status 409: {"succeeded":false,"data":null,"message":"...","errors":null,"path":"..."}`.
**Por qué duele:** El backend ya manda un `message` legible; volcar el JSON crudo es ruido y filtra detalles internos (`path`, estructura del envelope).
**Fix:** Extraer SOLO el `message` con el helper centralizado `api.ErrorMessage(respBody, statusCode)` (parsea `types.ErrorResponse`, con fallback al body acotado si no hay message). Lo usan `Login`, `ToggleFlowCore` y `push`. No duplicar el bloque de `json.Unmarshal(... errResp)` en cada método (eso era C6).

---

## H. Comentarios

### H1. Godoc faltante en exported types/funcs
**Trigger:** `func NewClient(baseURL string) *Client {` sin doc comment.
**Por qué duele:** No aparece en `go doc` ni en LSP hover. linter `golint`/`revive` se queja.
**Fix:** `// NewClient crea un cliente HTTP para hablar con el API de Oráculo.\nfunc NewClient(...)`.

### H2. Comentar lo obvio
**Trigger:** `// guarda el token` arriba de `saveToken(token)`.
**Fix:** Borrar. El nombre lo dice.

### H3. Comentar referencias a otros archivos del repo
**Trigger:** `// usado desde cmd/record.go`.
**Por qué duele:** Cambian las rutas, los números de línea rotan.
**Fix:** Borrar. El reader usa codegraph_callers / IDE goto-references.

---

## I. Type safety

### I1. `interface{}` / `any` en lugar de tipo concreto
**Trigger:** `func parse(data any) error`.
**Fix:** Generics (Go 1.18+): `func parse[T any](data T) error`. O tipo concreto si conocés el shape.

### I2. Type assertion sin OK pattern
**Trigger:** `s := v.(string)` que panica si v no es string.
**Fix:** `s, ok := v.(string); if !ok { return fmt.Errorf("expected string, got %T", v) }`.

### I3. Conversión de tipo silenciosa
**Trigger:** `int32(largeInt64)` truncando sin chequear overflow.
**Fix:** Validar rango antes de convertir.

---

## J. Workflow

### J1. Commit con código de debug
**Trigger:** `fmt.Println("DEBUG: ...")`, `// TODO: remove`.
**Fix:** Pre-commit hacer `git diff`.

### J2. `panic("not implemented")` en código mergado
**Fix:** Si no está implementado, no lo mergees. Si es placeholder, devuelve `errors.New("not implemented")` y tracé un issue.

### J3. Binarios commiteados (`oraculo-windows.exe`)
**Trigger:** Build artifacts en git.
**Por qué duele:** Repo crece, hash binario no diffea, hay que regenerar con cada cambio.
**Fix:** Build via release pipeline o `make`. Agregar a `.gitignore`.

### J4. NO incluir `Co-Authored-By: Claude` en commits
Eric lo pidió explícito.

---

## Cómo usar esta skill en práctica

1. Antes de agregar un comando Cobra: releé sección D.
2. Antes de modificar `internal/api/client.go`: releé secciones B y C.
3. Antes de commit: pre-commit checklist (J + git diff completo).
4. Si encontrás un antipatrón NUEVO sistemático, agregalo acá.

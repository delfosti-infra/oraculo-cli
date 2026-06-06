---
name: clean-architecture-patterns
description: Patrones de arquitectura aplicados en oraculo-cli (Go 1.26 + Cobra + custom HTTP client). Define el layout cmd/internal, separación entre comando y lógica, HTTP client, types, UI feedback. Usar al crear comandos nuevos, al agregar funcionalidad al cliente HTTP, o cuando dudás dónde poner una pieza nueva.
metadata:
  type: architecture-guide
  scope: oraculo-cli
---

# Architecture Patterns — Oráculo CLI (Go)

Guía aplicada de la arquitectura de oraculo-cli (Go 1.26 + Cobra + lipgloss). Decisiones concretas + razones.

Usar cuando:
- Vas a agregar un comando nuevo (qué archivos hacer)
- Vas a agregar un endpoint al cliente HTTP
- Dudás si algo va en `cmd/`, `internal/api/`, `internal/ui/`, o como helper aparte
- Refactorizás deuda

---

## 1. Layout del repo

```
oraculo-cli/
├── main.go               ← entry point. Solo llama a cmd.Execute()
├── cmd/                  ← comandos Cobra: un file por comando
│   ├── root.go           ← rootCmd, AddCommand de subcomandos
│   ├── login.go          ← oraculo login
│   ├── init.go           ← oraculo init
│   ├── record.go         ← oraculo record <name>
│   ├── push.go           ← oraculo push
│   ├── check.go          ← oraculo check
│   └── jira_meta.go      ← oraculo jira-meta
├── internal/             ← código no exportable a otros módulos
│   ├── api/
│   │   ├── client.go     ← HTTP client al core
│   │   └── types/        ← request/response types
│   │       └── user.type.go
│   └── ui/               ← output formateado
│       ├── banner.go
│       └── feedback.go
└── e2e/                  ← tests end-to-end
```

### Por qué `internal/`
Go enforza que packages bajo `internal/` solo pueden ser importados por código en el mismo módulo. Garantiza que esta API NO es consumida externamente.

### Por qué un file por comando
- Cada comando es independiente. Tocás `cmd/login.go` sin riesgo de afectar `cmd/push.go`.
- Cobra registra cada comando via `init()` en su file (`rootCmd.AddCommand(loginCmd)`).
- Si un comando crece >200 LOC, dividir en sub-files: `cmd/record.go` (binding Cobra) + `cmd/record_codegen.go` (helpers).

---

## 2. ¿Dónde vive cada pieza? — árbol de decisión

### "Necesito agregar un comando nuevo (`oraculo foo`)"
→ **`cmd/foo.go`** con:
- Una struct `fooCmd = &cobra.Command{...}` exportada solo si la usa root
- Flags como variables locales o adentro del comando
- `RunE` que parsea flags y llama a una función `runFoo(opts FooOpts) error`
- `init()` que registra: `rootCmd.AddCommand(fooCmd)`

### "Necesito agregar un endpoint al cliente HTTP del core"
→ **`internal/api/client.go`** (método sobre `*Client`).
→ **`internal/api/types/X.type.go`** (request/response structs).

### "Necesito mostrar un mensaje formateado al usuario"
→ **`internal/ui/feedback.go`** (funciones públicas: `ui.Success`, `ui.Step`, `ui.Error`).

### "Necesito un helper específico de un comando (parser de regex, slugify)"
→ **Local al `cmd/foo.go`** como función privada. Solo extraer a un package nuevo si lo usan ≥2 comandos.

### "Necesito un type compartido entre cliente HTTP y comando"
→ **`internal/api/types/X.type.go`**. Los comandos importan desde ahí.

### "Necesito un helper que no es HTTP ni UI (ej. spec parser, screenshot processor)"
→ **`internal/X/` package nuevo**. Patrón: cada cross-cutting helper en su propio package nombrado.

### "Necesito leer/escribir config global"
→ **`internal/config/` (a crear si no existe)** con un Loader + Saver. Hoy está disperso en cmd/init.go y cmd/login.go.

---

## 3. Cobra commands — la convención

### Estructura ideal
```go
package cmd

import (
    "github.com/Delfosti-Platform/oraculo-cli/internal/api"
    "github.com/Delfosti-Platform/oraculo-cli/internal/ui"
    "github.com/spf13/cobra"
)

// flags como variables del package (Cobra requiere esto para BindPFlag)
var fooFlag string
var fooNumberFlag int

var fooCmd = &cobra.Command{
    Use:     "foo <arg>",
    Short:   "Hace foo en el proyecto actual",
    Args:    cobra.ExactArgs(1),
    Example: "  oraculo foo bar\n  oraculo foo bar --flag x",
    RunE: func(cmd *cobra.Command, args []string) error {
        opts := fooOpts{
            Name:   args[0],
            Flag:   fooFlag,
            Number: fooNumberFlag,
        }
        return runFoo(opts)
    },
}

func init() {
    fooCmd.Flags().StringVar(&fooFlag, "flag", "", "descripción")
    fooCmd.Flags().IntVar(&fooNumberFlag, "n", 0, "descripción")
    rootCmd.AddCommand(fooCmd)
}

type fooOpts struct {
    Name   string
    Flag   string
    Number int
}

func runFoo(opts fooOpts) error {
    client := api.NewClient(loadAPIBaseURL())
    // ... lógica del comando
    ui.Success("foo completado")
    return nil
}
```

### Reglas duras
- `RunE` NUNCA tiene lógica. Solo parsea args/flags y llama a `runX(opts)`.
- `runX` recibe una struct `XOpts` (no múltiples params).
- `runX` devuelve `error`. `RunE` lo propaga.
- Mensajes al usuario van vía `internal/ui/`, no `fmt.Println` directo.
- Errores devueltos como `error`; root maneja la impresión a stderr y exit code.

### Args validation
- `cobra.ExactArgs(N)`, `cobra.MinimumNArgs(N)`, `cobra.MaximumNArgs(N)`, `cobra.RangeArgs(min, max)`.
- Si no validás args, el RunE puede panic con index out of range.

### Help y examples
- Todo comando tiene `Short` (1 línea, aparece en `oraculo --help`).
- Comandos con flags o casos no obvios tienen `Long` (multi-línea) y/o `Example`.

---

## 4. HTTP client — `internal/api/client.go`

### Estructura
```go
type Client struct {
    baseURL    string
    token      string  // si hay auth state
    httpClient *http.Client
}

func NewClient(baseURL string) *Client {
    return &Client{
        baseURL: baseURL,
        httpClient: &http.Client{ Timeout: 30 * time.Second },
    }
}

func (c *Client) WithToken(token string) *Client {
    c.token = token
    return c
}
```

### Reglas
- Un solo `*Client` per ejecución de comando (no reutilizado entre procesos).
- `Timeout` siempre. NUNCA `http.DefaultClient`.
- Cada endpoint del core es un método: `Login`, `PushFlow`, `UploadTrace`, `GetVariants`.
- Request/response shapes en `internal/api/types/`.
- Error responses parsed a `types.ErrorResponse` con mensaje legible.

### Patrón canónico de método
```go
func (c *Client) Login(email, password string) (*types.AuthSession, error) {
    body, err := json.Marshal(types.LoginRequest{...})
    if err != nil {
        return nil, fmt.Errorf("serializar login: %w", err)
    }

    resp, err := c.httpClient.Post(c.baseURL+"/auth/login", "application/json", bytes.NewBuffer(body))
    if err != nil {
        return nil, fmt.Errorf("conectar al api: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("leer respuesta: %w", err)
    }

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        var errResp types.ErrorResponse
        _ = json.Unmarshal(respBody, &errResp)
        if errResp.Message != "" {
            return nil, fmt.Errorf("login: %s", errResp.Message)
        }
        return nil, fmt.Errorf("login: status %d", resp.StatusCode)
    }

    var session types.AuthSession
    if err := json.Unmarshal(respBody, &session); err != nil {
        return nil, fmt.Errorf("parsear sesión: %w", err)
    }
    return &session, nil
}
```

### Cuándo extraer helper privado `c.do(...)`
Cuando 3+ métodos repitan el bloque marshal/POST/check status. Hacer:
```go
func (c *Client) do(method, path string, body any, result any) error
```
Cada método público queda 3-4 líneas.

### Multipart upload (screenshots, trace)
- Custom: `multipart.NewWriter` + manual.
- Timeout más largo (`60s` o configurable).
- Streaming para archivos grandes (no leer todo en memoria).

---

## 5. Types — `internal/api/types/`

### Convención de file naming
- `user.type.go` — `LoginRequest`, `AuthSession`, `User`
- `flow.type.go` — `FlowRequest`, `FlowResponse`
- `execution.type.go` — types de executions
- `error.type.go` — `ErrorResponse`

Un file por dominio.

### Struct tags
```go
type AuthSession struct {
    Token     string    `json:"token"`
    ExpiresAt time.Time `json:"expiresAt"`
    User      User      `json:"user"`
}
```
- `json:"camelCase"` para alinear con el backend (NestJS usa camelCase).
- `json:",omitempty"` cuando el campo es opcional en serialización.
- NO comentarios redundantes con el field name.

### Sin métodos en types (regla suave)
Los types son data. Lógica va en `internal/api/client.go` o helper packages. Excepción: validación simple (`func (r *LoginRequest) Validate() error`).

---

## 6. UI / feedback — `internal/ui/`

### Responsabilidades
- `banner.go` — banner ASCII art al `oraculo` sin args.
- `feedback.go` — funciones de output: `Success`, `Error`, `Step`, `Warn`, `Heading`.

### Por qué centralizar
- Style consistente entre comandos (colors, prefixes, spacing).
- Detección de terminal capabilities (TTY, color support) en un solo lugar.
- Cambios de design system (ej. switch de emojis a símbolos) en un solo file.

### Patrón
```go
// internal/ui/feedback.go
func Success(msg string) {
    fmt.Println(successStyle.Render("✓ "), msg)
}

func Step(label, value string) {
    fmt.Println(labelStyle.Render(label), arrowStyle.Render("→"), value)
}

func Error(msg string) {
    fmt.Fprintln(os.Stderr, errorStyle.Render("✗ "), msg)
}
```

### Reglas
- Funciones simples, sin estado.
- `Error` y `Warn` a stderr; el resto a stdout.
- Estilos definidos arriba del file con `lipgloss.NewStyle()`.
- Símbolos consistentes: ✓ success, ✗ error, → step, ⚠ warn, ◆ heading.

---

## 7. Config & state

### Archivos de config
- `oraculo.config.json` — config local del proyecto (API URL, project ref). Generado por `oraculo init`.
- `~/.oraculo/credentials.json` (TBD) — token de auth, separado del proyecto.

### Reglas
- Config del proyecto va en cwd o se busca hacia arriba (como `package.json`).
- Credentials van en `$HOME` con permisos `0600`.
- NO mezclar: el proyecto puede compartirse en git, las credentials no.

### Cuándo extraer un package `internal/config/`
Cuando ≥2 comandos leen/escriben el mismo file. Hoy está disperso (init y login leen/escriben config), lo cual es deuda candidata a refactor.

---

## 8. Errores y exit codes

### Cadena de propagación
```
runFoo(opts) error → RunE → cmd.Execute() → Execute() → main()
```
- Cada nivel WRAPpea con contexto: `fmt.Errorf("foo: %w", err)`.
- `cmd.Execute()` (en `cmd/root.go`) imprime a stderr y hace `os.Exit(1)` si hay error.
- Exit code 0 = success, 1 = error genérico. Si querés exit codes específicos, definir tipo `ExitError` con campo `Code int`.

### Sentinel errors públicos
```go
// internal/api/errors.go (a crear si necesario)
var (
    ErrUnauthorized = errors.New("token inválido o expirado")
    ErrConflict     = errors.New("recurso ya existe")
)
```
Permite a los comandos hacer `errors.Is(err, api.ErrUnauthorized)` y ofrecer remediation específica ("corré `oraculo login`").

---

## 9. Testing — `e2e/`

### Estructura
- `e2e/` para tests end-to-end del CLI.
- Tests unitarios al lado del file (`client_test.go` junto a `client.go`).
- Por convención Go, NO carpeta `tests/` separada.

### Patrón
- Test del HTTP client: usar `httptest.NewServer` para fakear el core.
- Test de comando: invocar `cmd.Execute()` con args y assert sobre output capturado.

---

## 10. Layout flat — no anidar comandos por categoría

```
cmd/
├── login.go
├── init.go
├── record.go
├── push.go
├── check.go
└── jira_meta.go
```
- ❌ `cmd/jira/meta.go`, `cmd/jira/link.go` (nested por categoría)
- ✅ `cmd/jira_meta.go`, `cmd/jira_link.go` con prefix en el filename

**Por qué:** Go convención es flat dentro de un package. Si necesitás group conceptual, usá prefix en el name. Cobra acepta sub-comandos via `parentCmd.AddCommand(childCmd)` independientemente de la estructura física.

---

## 11. Checklist antes de mergear

- [ ] Cada comando nuevo está en su propio `cmd/X.go`.
- [ ] `RunE` solo parsea, lógica en `runX(opts)` separada.
- [ ] `Args:` validator definido.
- [ ] Error wrapping con `fmt.Errorf("... %w", err)`.
- [ ] HTTP client tiene timeout.
- [ ] `defer resp.Body.Close()` después del check de error.
- [ ] Mensajes al usuario vía `internal/ui/`, no `fmt.Println` directo.
- [ ] Errores a stderr, output a stdout.
- [ ] Types nuevos en `internal/api/types/X.type.go`.
- [ ] godoc en exported funcs/types.
- [ ] `go vet ./...` limpio.
- [ ] `go build ./...` ok.

---

## Cómo usar esta skill en práctica

- Al agregar un comando: releer sección 3 (Cobra) + 4 (HTTP client) si toca el core.
- Al modificar el HTTP client: sección 4 + 5 (types).
- Al revisar un PR: checklist sección 11.
- Si encontrás caso no cubierto, agregalo.

package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

type mockCIClient struct {
	flows       []types.FlowSummary
	listErr     error
	createFn    func(flowRefId string) (*types.Execution, error)
	getFn       func(executionRefId string, call int) (*types.Execution, error)
	createCalls []string
	getCalls    int
}

func (m *mockCIClient) ListFlows(token, projectRefId string) ([]types.FlowSummary, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.flows, nil
}

func (m *mockCIClient) CreateExecution(token, flowRefId string, request types.CreateExecutionRequest) (*types.Execution, error) {
	m.createCalls = append(m.createCalls, flowRefId)
	return m.createFn(flowRefId)
}

func (m *mockCIClient) GetExecution(token, executionRefId string) (*types.Execution, error) {
	m.getCalls++
	return m.getFn(executionRefId, m.getCalls)
}

func connectionError() error {
	return fmt.Errorf("no se pudo conectar con el API: %w", &url.Error{
		Op:  "Get",
		URL: "http://localhost:3000",
		Err: errors.New("connection refused"),
	})
}

func silentPrinter() *ciPrinter {
	return newCIPrinter(false)
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func ciTestFlows() []types.FlowSummary {
	return []types.FlowSummary{
		{RefId: "flow-1", Slug: "login", Name: "Login", ExecutableAsWorkflow: true},
		{RefId: "flow-2", Slug: "checkout", Name: "Checkout", ExecutableAsWorkflow: true,
			WorkflowParams: []types.WorkflowParam{{Name: "user"}, {Name: "password"}}},
		{RefId: "flow-3", Slug: "draft", Name: "Draft", ExecutableAsWorkflow: false},
		{RefId: "flow-4", Slug: "signup", Name: "Signup", ExecutableAsWorkflow: true},
	}
}

func TestCICommandFlagDefaults(t *testing.T) {
	cases := []struct {
		flag string
		want string
	}{
		{"wait", "true"},
		{"timeout", "15m0s"},
		{"output", "text"},
		{"out", ""},
		{"fail-fast", "false"},
		{"api-key", ""},
	}
	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			flag := ciCmd.Flags().Lookup(c.flag)
			if flag == nil {
				t.Fatalf("flag --%s no registrado", c.flag)
			}
			if flag.DefValue != c.want {
				t.Errorf("default de --%s = %q, se esperaba %q", c.flag, flag.DefValue, c.want)
			}
		})
	}
}

func TestCIFlagParsing(t *testing.T) {
	flags := ciCmd.Flags()
	t.Cleanup(func() {
		_ = flags.Set("wait", "true")
		_ = flags.Set("timeout", "15m")
		_ = flags.Set("output", "text")
		_ = flags.Set("out", "")
		_ = flags.Set("fail-fast", "false")
		_ = flags.Set("api-key", "")
	})

	args := []string{
		"--output", "junit",
		"--out", "report.xml",
		"--timeout", "90s",
		"--wait=false",
		"--fail-fast",
		"--api-key", "oraculo_sk_abc",
	}
	if err := flags.Parse(args); err != nil {
		t.Fatalf("parse falló: %v", err)
	}

	if ciOutputFlag != "junit" {
		t.Errorf("output = %q, se esperaba junit", ciOutputFlag)
	}
	if ciOutFlag != "report.xml" {
		t.Errorf("out = %q, se esperaba report.xml", ciOutFlag)
	}
	if ciTimeoutFlag != 90*time.Second {
		t.Errorf("timeout = %s, se esperaba 90s", ciTimeoutFlag)
	}
	if ciWaitFlag {
		t.Error("wait debía quedar en false")
	}
	if !ciFailFastFlag {
		t.Error("fail-fast debía quedar en true")
	}
	if ciAPIKeyFlag != "oraculo_sk_abc" {
		t.Errorf("api-key = %q, se esperaba oraculo_sk_abc", ciAPIKeyFlag)
	}
}

func TestValidateCIOpts(t *testing.T) {
	cases := []struct {
		name    string
		opts    ciOpts
		wantErr string
	}{
		{"text válido", ciOpts{Output: "text", Timeout: time.Minute, Wait: true}, ""},
		{"json válido", ciOpts{Output: "json", Timeout: time.Minute, Wait: true}, ""},
		{"junit con out", ciOpts{Output: "junit", OutFile: "r.xml", Timeout: time.Minute, Wait: true}, ""},
		{"output inválido", ciOpts{Output: "yaml", Timeout: time.Minute, Wait: true}, "--output inválido"},
		{"out con text", ciOpts{Output: "text", OutFile: "r.xml", Timeout: time.Minute, Wait: true}, "--out requiere"},
		{"timeout cero", ciOpts{Output: "text", Timeout: 0, Wait: true}, "--timeout debe ser mayor"},
		{"fail-fast sin wait", ciOpts{Output: "text", Timeout: time.Minute, Wait: false, FailFast: true}, "--fail-fast requiere --wait"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCIOpts(c.opts)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("error inesperado: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, debía contener %q", err, c.wantErr)
			}
		})
	}
}

func TestSelectCITargets(t *testing.T) {
	cases := []struct {
		name        string
		slugs       []string
		wantSlugs   []string
		wantSkipped []string
		wantErr     string
	}{
		{"sin args filtra ejecutables sin params", nil, []string{"login", "signup"}, []string{"checkout"}, ""},
		{"slug explícito", []string{"login"}, []string{"login"}, nil, ""},
		{"slug se normaliza", []string{"Login"}, []string{"login"}, nil, ""},
		{"slugs duplicados se dedupean", []string{"login", "login"}, []string{"login"}, nil, ""},
		{"varios slugs conservan orden", []string{"signup", "login"}, []string{"signup", "login"}, nil, ""},
		{"slug inexistente", []string{"ghost"}, nil, nil, "no existe en el proyecto"},
		{"flow no promovido", []string{"draft"}, nil, nil, "no está promovido como workflow"},
		{"workflow con params", []string{"checkout"}, nil, nil, "requiere parámetros (user, password)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			targets, skipped, err := selectCITargets(ciTestFlows(), c.slugs)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, debía contener %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			var gotSlugs []string
			for _, target := range targets {
				gotSlugs = append(gotSlugs, target.Slug)
			}
			if strings.Join(gotSlugs, ",") != strings.Join(c.wantSlugs, ",") {
				t.Errorf("targets = %v, se esperaba %v", gotSlugs, c.wantSlugs)
			}
			if strings.Join(skipped, ",") != strings.Join(c.wantSkipped, ",") {
				t.Errorf("skipped = %v, se esperaba %v", skipped, c.wantSkipped)
			}
		})
	}
}

func TestResolveCIToken(t *testing.T) {
	t.Run("flag gana sobre env", func(t *testing.T) {
		t.Setenv(ciAPIKeyEnvVar, "oraculo_sk_env")
		token, source, err := resolveCIToken("oraculo_sk_flag")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if token != "oraculo_sk_flag" || source != ciTokenSourceFlag {
			t.Errorf("token = %q source = %q", token, source)
		}
	})

	t.Run("env cuando no hay flag", func(t *testing.T) {
		t.Setenv(ciAPIKeyEnvVar, "oraculo_sk_env")
		token, source, err := resolveCIToken("")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if token != "oraculo_sk_env" || source != ciTokenSourceEnv {
			t.Errorf("token = %q source = %q", token, source)
		}
	})

	t.Run("cae al token de login", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOME", home)
		t.Setenv(ciAPIKeyEnvVar, "")
		if err := os.MkdirAll(filepath.Join(home, ".oraculo"), 0700); err != nil {
			t.Fatalf("no se pudo crear ~/.oraculo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, ".oraculo", "token"), []byte("jwt-token\n"), 0600); err != nil {
			t.Fatalf("no se pudo escribir token: %v", err)
		}
		token, source, err := resolveCIToken("")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if token != "jwt-token" || source != ciTokenSourceLogin {
			t.Errorf("token = %q source = %q", token, source)
		}
	})

	t.Run("sin credenciales", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOME", home)
		t.Setenv(ciAPIKeyEnvVar, "")
		_, _, err := resolveCIToken("")
		if err == nil || !strings.Contains(err.Error(), "sin credenciales") {
			t.Fatalf("err = %v, debía contener 'sin credenciales'", err)
		}
	})
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"error simple", errors.New("boom"), 1},
		{"código 2", withExitCode(2, errors.New("infra")), 2},
		{"código 2 ya reportado", withExitCode(2, ui.ErrAlreadyReported), 2},
		{"código 1 explícito", withExitCode(1, errors.New("flow")), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exitCodeFor(c.err); got != c.want {
				t.Errorf("exitCodeFor = %d, se esperaba %d", got, c.want)
			}
		})
	}

	t.Run("nil no se envuelve", func(t *testing.T) {
		if withExitCode(2, nil) != nil {
			t.Error("withExitCode(2, nil) debía devolver nil")
		}
	})

	t.Run("preserva ErrAlreadyReported", func(t *testing.T) {
		err := withExitCode(2, ui.ErrAlreadyReported)
		if !errors.Is(err, ui.ErrAlreadyReported) {
			t.Error("la cadena debía preservar ui.ErrAlreadyReported")
		}
	})
}

func TestApplyExecutionToRun(t *testing.T) {
	cases := []struct {
		name        string
		execution   types.Execution
		wantOutcome string
		wantMessage string
		wantStep    *int
	}{
		{
			name:        "success pasa",
			execution:   types.Execution{Status: types.ExecutionStatusSuccess, DurationMs: int64Ptr(1200)},
			wantOutcome: ciOutcomePassed,
		},
		{
			name:        "success con mismatch falla",
			execution:   types.Execution{Status: types.ExecutionStatusSuccess, MatchStatus: types.ExecutionMatchStatusMismatch},
			wantOutcome: ciOutcomeFailed,
			wantMessage: "MISMATCH",
		},
		{
			name: "failed con contexto",
			execution: types.Execution{
				Status:       types.ExecutionStatusFailed,
				ErrorMessage: "TimeoutError",
				FailureContext: &types.ExecutionFailureContext{
					Error:           "TimeoutError",
					URL:             "https://demo.test/checkout",
					FailedStepIndex: intPtr(3),
				},
			},
			wantOutcome: ciOutcomeFailed,
			wantMessage: "TimeoutError",
			wantStep:    intPtr(3),
		},
		{
			name:        "cancelled sin mensaje usa default",
			execution:   types.Execution{Status: types.ExecutionStatusCancelled},
			wantOutcome: ciOutcomeFailed,
			wantMessage: "CANCELLED",
		},
		{
			name:        "running sigue pendiente",
			execution:   types.Execution{Status: types.ExecutionStatusRunning},
			wantOutcome: ciOutcomePending,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := &ciFlowRun{Slug: "login", Outcome: ciOutcomePending}
			applyExecutionToRun(run, &c.execution)
			if run.Outcome != c.wantOutcome {
				t.Errorf("outcome = %q, se esperaba %q", run.Outcome, c.wantOutcome)
			}
			if c.wantMessage != "" && !strings.Contains(run.ErrorMessage, c.wantMessage) {
				t.Errorf("errorMessage = %q, debía contener %q", run.ErrorMessage, c.wantMessage)
			}
			if c.wantStep != nil {
				if run.FailedStep == nil || *run.FailedStep != *c.wantStep {
					t.Errorf("failedStep = %v, se esperaba %d", run.FailedStep, *c.wantStep)
				}
			}
		})
	}
}

func TestWaitForCIRuns(t *testing.T) {
	deadline := func() time.Time { return time.Now().Add(time.Second) }

	t.Run("pasa tras varios polls", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				if call < 3 {
					return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusRunning}, nil
				}
				return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusSuccess}, nil
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		err := waitForCIRuns(client, "token", runs, time.Millisecond, deadline(), silentPrinter())
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if runs[0].Outcome != ciOutcomePassed {
			t.Errorf("outcome = %q, se esperaba passed", runs[0].Outcome)
		}
		if client.getCalls != 3 {
			t.Errorf("getCalls = %d, se esperaba 3", client.getCalls)
		}
	})

	t.Run("fallo del flow no es error de infra", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusFailed, ErrorMessage: "boom"}, nil
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		if err := waitForCIRuns(client, "token", runs, time.Millisecond, deadline(), silentPrinter()); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if runs[0].Outcome != ciOutcomeFailed {
			t.Errorf("outcome = %q, se esperaba failed", runs[0].Outcome)
		}
	})

	t.Run("timeout global es error código 2", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusRunning}, nil
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		err := waitForCIRuns(client, "token", runs, time.Millisecond, time.Now().Add(-time.Second), silentPrinter())
		if err == nil {
			t.Fatal("se esperaba error por timeout")
		}
		if exitCodeFor(err) != ciExitInfra {
			t.Errorf("exit code = %d, se esperaba %d", exitCodeFor(err), ciExitInfra)
		}
	})

	t.Run("errores de conexión transitorios se reintentan", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				if call <= 2 {
					return nil, connectionError()
				}
				return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusSuccess}, nil
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		if err := waitForCIRuns(client, "token", runs, time.Millisecond, deadline(), silentPrinter()); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if runs[0].Outcome != ciOutcomePassed {
			t.Errorf("outcome = %q, se esperaba passed", runs[0].Outcome)
		}
	})

	t.Run("conexión perdida persistente corta con código 2", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				return nil, connectionError()
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		err := waitForCIRuns(client, "token", runs, time.Millisecond, deadline(), silentPrinter())
		if err == nil {
			t.Fatal("se esperaba error de conexión")
		}
		if exitCodeFor(err) != ciExitInfra {
			t.Errorf("exit code = %d, se esperaba %d", exitCodeFor(err), ciExitInfra)
		}
		if client.getCalls != ciMaxConsecutivePollErrors {
			t.Errorf("getCalls = %d, se esperaba %d", client.getCalls, ciMaxConsecutivePollErrors)
		}
	})

	t.Run("error del API no conexión corta de inmediato", func(t *testing.T) {
		client := &mockCIClient{
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				return nil, errors.New("Execution 'x' no encontrado")
			},
		}
		runs := []*ciFlowRun{{Slug: "login", ExecutionRefId: "exec-1", Outcome: ciOutcomePending}}
		err := waitForCIRuns(client, "token", runs, time.Millisecond, deadline(), silentPrinter())
		if err == nil || exitCodeFor(err) != ciExitInfra {
			t.Fatalf("err = %v (code %d), se esperaba infra", err, exitCodeFor(err))
		}
		if client.getCalls != 1 {
			t.Errorf("getCalls = %d, se esperaba 1", client.getCalls)
		}
	})
}

func TestRunCISequentialFailFast(t *testing.T) {
	client := &mockCIClient{
		createFn: func(flowRefId string) (*types.Execution, error) {
			return &types.Execution{RefId: "exec-" + flowRefId, Status: types.ExecutionStatusPending}, nil
		},
		getFn: func(executionRefId string, call int) (*types.Execution, error) {
			return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusFailed, ErrorMessage: "rompió"}, nil
		},
	}
	runs := newCIRuns([]types.FlowSummary{
		{RefId: "flow-1", Slug: "login", Name: "Login"},
		{RefId: "flow-2", Slug: "signup", Name: "Signup"},
		{RefId: "flow-3", Slug: "checkout", Name: "Checkout"},
	})

	err := runCISequential(client, "token", runs, time.Millisecond, time.Now().Add(time.Second), silentPrinter())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(client.createCalls) != 1 {
		t.Errorf("createCalls = %v, se esperaba solo flow-1", client.createCalls)
	}
	if runs[0].Outcome != ciOutcomeFailed {
		t.Errorf("run 0 = %q, se esperaba failed", runs[0].Outcome)
	}
	for _, run := range runs[1:] {
		if run.Outcome != ciOutcomeSkipped || run.SkipReason != ciSkipReasonFailFast {
			t.Errorf("run '%s' = %q (%q), se esperaba skipped por fail-fast", run.Slug, run.Outcome, run.SkipReason)
		}
	}
}

func TestRunCIAggregate(t *testing.T) {
	t.Run("sin wait encola todo y no pollea", func(t *testing.T) {
		client := &mockCIClient{
			createFn: func(flowRefId string) (*types.Execution, error) {
				return &types.Execution{RefId: "exec-" + flowRefId, Status: types.ExecutionStatusPending}, nil
			},
		}
		runs := newCIRuns([]types.FlowSummary{
			{RefId: "flow-1", Slug: "login"},
			{RefId: "flow-4", Slug: "signup"},
		})
		err := runCIAggregate(client, "token", runs, false, time.Millisecond, time.Now().Add(time.Second), silentPrinter())
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(client.createCalls) != 2 || client.getCalls != 0 {
			t.Errorf("createCalls = %v getCalls = %d", client.createCalls, client.getCalls)
		}
		for _, run := range runs {
			if run.Outcome != ciOutcomePending {
				t.Errorf("run '%s' = %q, se esperaba pending", run.Slug, run.Outcome)
			}
		}
	})

	t.Run("rechazo del API al crear marca el flow como fallido", func(t *testing.T) {
		client := &mockCIClient{
			createFn: func(flowRefId string) (*types.Execution, error) {
				if flowRefId == "flow-4" {
					return nil, errors.New("La sesión del proyecto expiró")
				}
				return &types.Execution{RefId: "exec-" + flowRefId, Status: types.ExecutionStatusPending}, nil
			},
			getFn: func(executionRefId string, call int) (*types.Execution, error) {
				return &types.Execution{RefId: executionRefId, Status: types.ExecutionStatusSuccess}, nil
			},
		}
		runs := newCIRuns([]types.FlowSummary{
			{RefId: "flow-1", Slug: "login"},
			{RefId: "flow-4", Slug: "signup"},
		})
		err := runCIAggregate(client, "token", runs, true, time.Millisecond, time.Now().Add(time.Second), silentPrinter())
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if runs[0].Outcome != ciOutcomePassed {
			t.Errorf("login = %q, se esperaba passed", runs[0].Outcome)
		}
		if runs[1].Outcome != ciOutcomeFailed || !strings.Contains(runs[1].ErrorMessage, "no se pudo crear la ejecución") {
			t.Errorf("signup = %q (%q), se esperaba failed por create", runs[1].Outcome, runs[1].ErrorMessage)
		}
	})

	t.Run("error de conexión al crear es infra", func(t *testing.T) {
		client := &mockCIClient{
			createFn: func(flowRefId string) (*types.Execution, error) {
				return nil, connectionError()
			},
		}
		runs := newCIRuns([]types.FlowSummary{{RefId: "flow-1", Slug: "login"}})
		err := runCIAggregate(client, "token", runs, true, time.Millisecond, time.Now().Add(time.Second), silentPrinter())
		if err == nil || exitCodeFor(err) != ciExitInfra {
			t.Fatalf("err = %v, se esperaba infra código 2", err)
		}
	})
}

func TestExecuteCI(t *testing.T) {
	baseOpts := ciOpts{Output: ciOutputText, Timeout: time.Second, Wait: true}

	t.Run("fallo listando flows es infra", func(t *testing.T) {
		client := &mockCIClient{listErr: errors.New("401 unauthorized")}
		_, _, err := executeCI(client, "token", "proj-1", baseOpts, silentPrinter())
		if err == nil || exitCodeFor(err) != ciExitInfra {
			t.Fatalf("err = %v, se esperaba infra código 2", err)
		}
	})

	t.Run("sin workflows elegibles es infra", func(t *testing.T) {
		client := &mockCIClient{flows: []types.FlowSummary{{Slug: "draft", ExecutableAsWorkflow: false}}}
		_, _, err := executeCI(client, "token", "proj-1", baseOpts, silentPrinter())
		if err == nil || exitCodeFor(err) != ciExitInfra {
			t.Fatalf("err = %v, se esperaba infra código 2", err)
		}
	})

	t.Run("camino feliz arma el veredicto", func(t *testing.T) {
		client := &mockCIClient{
			flows: ciTestFlows(),
			createFn: func(flowRefId string) (*types.Execution, error) {
				return &types.Execution{
					RefId:      "exec-" + flowRefId,
					Status:     types.ExecutionStatusSuccess,
					DurationMs: int64Ptr(1500),
				}, nil
			},
		}
		runs, verdict, err := executeCI(client, "token", "proj-1", baseOpts, silentPrinter())
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(runs) != 2 {
			t.Fatalf("runs = %d, se esperaban 2 (login y signup)", len(runs))
		}
		if verdict.Verdict != ciVerdictPass || verdict.Passed != 2 || verdict.Total != 2 {
			t.Errorf("verdict = %+v, se esperaba PASS 2/2", verdict)
		}
		if strings.Join(verdict.Skipped, ",") != "checkout" {
			t.Errorf("skipped = %v, se esperaba [checkout]", verdict.Skipped)
		}
	})
}

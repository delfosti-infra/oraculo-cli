package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	ciPollInterval             = 5 * time.Second
	ciDefaultTimeout           = 15 * time.Minute
	ciAPIKeyEnvVar             = "ORACULO_API_KEY"
	ciAPIKeyPrefix             = "oraculo_sk_"
	ciTriggerNote              = "oraculo ci"
	ciMaxConsecutivePollErrors = 3
)

const (
	ciExitFlowFailed = 1
	ciExitInfra      = 2
)

const (
	ciOutputText  = "text"
	ciOutputJSON  = "json"
	ciOutputJUnit = "junit"
)

const (
	ciOutcomePending = "pending"
	ciOutcomePassed  = "passed"
	ciOutcomeFailed  = "failed"
	ciOutcomeSkipped = "skipped"
)

const (
	ciTokenSourceFlag  = "api-key (--api-key)"
	ciTokenSourceEnv   = "api-key (" + ciAPIKeyEnvVar + ")"
	ciTokenSourceLogin = "sesión de oraculo login"
)

const ciSkipReasonFailFast = "fail-fast"

var (
	ciWaitFlag     bool
	ciTimeoutFlag  time.Duration
	ciOutputFlag   string
	ciOutFlag      string
	ciFailFastFlag bool
	ciAPIKeyFlag   string
)

type ciClient interface {
	ListFlows(token, projectRefId string) ([]types.FlowSummary, error)
	CreateExecution(token, flowRefId string, request types.CreateExecutionRequest) (*types.Execution, error)
	GetExecution(token, executionRefId string) (*types.Execution, error)
}

type ciOpts struct {
	Slugs    []string
	Wait     bool
	Timeout  time.Duration
	Output   string
	OutFile  string
	FailFast bool
	APIKey   string
}

type ciFlowRun struct {
	Slug           string
	Name           string
	FlowRefId      string
	ExecutionRefId string
	Outcome        string
	Status         string
	MatchStatus    string
	ErrorMessage   string
	FailedStep     *int
	FailureURL     string
	DurationMs     *int64
	SkipReason     string
}

var ciCmd = &cobra.Command{
	Use:   "ci [flow-slug...]",
	Short: "Corre los workflows del proyecto como gate E2E para pipelines",
	Long: `Ejecuta los workflows del proyecto activo y falla si alguno rompe.
Sin argumentos corre todos los workflows sin parámetros; con slugs corre solo esos.

Exit codes: 0 todo en verde · 1 algún flow falló · 2 error de infraestructura/CLI.`,
	Example: `  oraculo ci
  oraculo ci login checkout
  oraculo ci --output junit --out report.xml
  oraculo ci --output json --fail-fast
  ORACULO_API_KEY=oraculo_sk_... oraculo ci`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := ciOpts{
			Slugs:    args,
			Wait:     ciWaitFlag,
			Timeout:  ciTimeoutFlag,
			Output:   ciOutputFlag,
			OutFile:  ciOutFlag,
			FailFast: ciFailFastFlag,
			APIKey:   ciAPIKeyFlag,
		}
		return runCI(opts)
	},
}

func init() {
	ciCmd.Flags().BoolVar(&ciWaitFlag, "wait", true, "Espera el resultado de cada ejecución (poll cada 5s)")
	ciCmd.Flags().DurationVar(&ciTimeoutFlag, "timeout", ciDefaultTimeout, "Timeout global de espera (ej. 10m, 90s)")
	ciCmd.Flags().StringVar(&ciOutputFlag, "output", ciOutputText, "Formato de salida: text, json o junit")
	ciCmd.Flags().StringVar(&ciOutFlag, "out", "", "Escribe el reporte json/junit a un archivo en lugar de stdout")
	ciCmd.Flags().BoolVar(&ciFailFastFlag, "fail-fast", false, "Corta al primer fallo y omite los workflows restantes")
	ciCmd.Flags().StringVar(&ciAPIKeyFlag, "api-key", "", "Service API key (oraculo_sk_...); si falta usa "+ciAPIKeyEnvVar+" o la sesión de oraculo login")
	rootCmd.AddCommand(ciCmd)
}

func runCI(opts ciOpts) error {
	if err := validateCIOpts(opts); err != nil {
		return withExitCode(ciExitInfra, ui.Fail("%s", err))
	}

	config, err := loadOraculoConfig()
	if err != nil {
		return withExitCode(ciExitInfra, ui.Fail("%s", err))
	}
	if config.RefId == "" {
		return withExitCode(ciExitInfra, ui.Fail("Falta el campo `refId` en oraculo.config.json. Corre 'oraculo init' para seleccionar el proyecto."))
	}

	token, tokenSource, err := resolveCIToken(opts.APIKey)
	if err != nil {
		return withExitCode(ciExitInfra, ui.Fail("%s", err))
	}
	if tokenSource != ciTokenSourceLogin && !strings.HasPrefix(token, ciAPIKeyPrefix) {
		ui.PrintWarning(fmt.Sprintf("La API key no empieza con '%s'; se enviará igual, pero revisa que sea una service key válida", ciAPIKeyPrefix))
	}

	printer := newCIPrinter(opts.Output == ciOutputText || opts.OutFile != "")
	printer.Header(config.Project, tokenSource)

	client := api.NewClient(appconfig.ResolveAPIURL("", config.APIURL))
	runs, verdict, err := executeCI(client, token, config.RefId, opts, printer)
	if err != nil {
		return err
	}

	if err := emitCIReport(opts, ciProjectLabel(config.Project), runs, verdict, printer); err != nil {
		return withExitCode(ciExitInfra, printer.Fail("%s", err))
	}

	if verdict.Failed > 0 {
		return withExitCode(ciExitFlowFailed, printer.Fail("%s", ciSummaryLabel(verdict)))
	}
	printer.Success(ciSummaryLabel(verdict))
	return nil
}

func validateCIOpts(opts ciOpts) error {
	switch opts.Output {
	case ciOutputText, ciOutputJSON, ciOutputJUnit:
	default:
		return fmt.Errorf("--output inválido '%s': usa text, json o junit", opts.Output)
	}
	if opts.OutFile != "" && opts.Output == ciOutputText {
		return fmt.Errorf("--out requiere --output json o junit")
	}
	if opts.Timeout <= 0 {
		return fmt.Errorf("--timeout debe ser mayor a 0")
	}
	if opts.FailFast && !opts.Wait {
		return fmt.Errorf("--fail-fast requiere --wait para conocer el resultado de cada flow")
	}
	return nil
}

func resolveCIToken(apiKeyFlag string) (string, string, error) {
	if key := strings.TrimSpace(apiKeyFlag); key != "" {
		return key, ciTokenSourceFlag, nil
	}
	if key := strings.TrimSpace(os.Getenv(ciAPIKeyEnvVar)); key != "" {
		return key, ciTokenSourceEnv, nil
	}
	token, err := appconfig.LoadToken()
	if err != nil {
		return "", "", fmt.Errorf("sin credenciales: pasa --api-key, define %s o corre 'oraculo login' (%w)", ciAPIKeyEnvVar, err)
	}
	return token, ciTokenSourceLogin, nil
}

func executeCI(client ciClient, token, projectRefId string, opts ciOpts, printer *ciPrinter) ([]*ciFlowRun, ciVerdict, error) {
	flows, err := client.ListFlows(token, projectRefId)
	if err != nil {
		return nil, ciVerdict{}, withExitCode(ciExitInfra, printer.Fail("No se pudieron listar los flows del proyecto: %s", err))
	}

	targets, paramSkipped, err := selectCITargets(flows, opts.Slugs)
	if err != nil {
		return nil, ciVerdict{}, withExitCode(ciExitInfra, printer.Fail("%s", err))
	}
	for _, slug := range paramSkipped {
		printer.Warning(fmt.Sprintf("'%s' requiere parámetros; omitido (córrelo desde el panel con sus valores)", slug))
	}
	if len(targets) == 0 {
		return nil, ciVerdict{}, withExitCode(ciExitInfra, printer.Fail("No hay workflows ejecutables sin parámetros en el proyecto. Promueve un flow como workflow o pasa slugs explícitos."))
	}
	printer.Step(fmt.Sprintf("%d workflow(s) a ejecutar", len(targets)))

	runs := newCIRuns(targets)
	deadline := time.Now().Add(opts.Timeout)
	if opts.FailFast {
		err = runCISequential(client, token, runs, ciPollInterval, deadline, printer)
	} else {
		err = runCIAggregate(client, token, runs, opts.Wait, ciPollInterval, deadline, printer)
	}
	if err != nil {
		return nil, ciVerdict{}, err
	}
	return runs, buildVerdict(runs, paramSkipped), nil
}

func selectCITargets(flows []types.FlowSummary, slugs []string) ([]types.FlowSummary, []string, error) {
	if len(slugs) == 0 {
		var targets []types.FlowSummary
		var skipped []string
		for _, flow := range flows {
			if !flow.ExecutableAsWorkflow {
				continue
			}
			if len(flow.WorkflowParams) > 0 {
				skipped = append(skipped, flow.Slug)
				continue
			}
			targets = append(targets, flow)
		}
		return targets, skipped, nil
	}

	bySlug := make(map[string]types.FlowSummary, len(flows))
	for _, flow := range flows {
		bySlug[flow.Slug] = flow
	}
	var targets []types.FlowSummary
	seen := map[string]bool{}
	for _, raw := range slugs {
		slug := slugify(raw)
		if seen[slug] {
			continue
		}
		seen[slug] = true
		flow, ok := bySlug[slug]
		if !ok {
			return nil, nil, fmt.Errorf("el flow '%s' no existe en el proyecto", slug)
		}
		if !flow.ExecutableAsWorkflow {
			return nil, nil, fmt.Errorf("el flow '%s' no está promovido como workflow; promuévelo desde el panel", slug)
		}
		if len(flow.WorkflowParams) > 0 {
			return nil, nil, fmt.Errorf("el workflow '%s' requiere parámetros (%s); oraculo ci solo corre workflows sin parámetros", slug, workflowParamNames(flow))
		}
		targets = append(targets, flow)
	}
	return targets, nil, nil
}

func workflowParamNames(flow types.FlowSummary) string {
	names := make([]string, len(flow.WorkflowParams))
	for i, param := range flow.WorkflowParams {
		names[i] = param.Name
	}
	return strings.Join(names, ", ")
}

func newCIRuns(targets []types.FlowSummary) []*ciFlowRun {
	runs := make([]*ciFlowRun, len(targets))
	for i, flow := range targets {
		runs[i] = &ciFlowRun{
			Slug:      flow.Slug,
			Name:      flow.Name,
			FlowRefId: flow.RefId,
			Outcome:   ciOutcomePending,
		}
	}
	return runs
}

func runCIAggregate(
	client ciClient,
	token string,
	runs []*ciFlowRun,
	wait bool,
	interval time.Duration,
	deadline time.Time,
	printer *ciPrinter,
) error {
	for _, run := range runs {
		if err := createCIExecution(client, token, run, printer); err != nil {
			return err
		}
		if run.Outcome == ciOutcomeFailed {
			printer.Result(run)
		} else {
			printer.Enqueued(run)
		}
	}
	if !wait {
		return nil
	}
	return waitForCIRuns(client, token, runs, interval, deadline, printer)
}

func runCISequential(
	client ciClient,
	token string,
	runs []*ciFlowRun,
	interval time.Duration,
	deadline time.Time,
	printer *ciPrinter,
) error {
	for i, run := range runs {
		if err := createCIExecution(client, token, run, printer); err != nil {
			return err
		}
		if run.Outcome == ciOutcomePending {
			if err := waitForCIRuns(client, token, runs[i:i+1], interval, deadline, printer); err != nil {
				return err
			}
		} else {
			printer.Result(run)
		}
		if run.Outcome == ciOutcomeFailed {
			markCISkippedFrom(runs, i+1, printer)
			return nil
		}
	}
	return nil
}

func createCIExecution(client ciClient, token string, run *ciFlowRun, printer *ciPrinter) error {
	execution, err := client.CreateExecution(token, run.FlowRefId, types.CreateExecutionRequest{
		ParamValues: map[string]string{},
		TriggerNote: ciTriggerNote,
	})
	if err != nil {
		if api.IsConnectionError(err) {
			return withExitCode(ciExitInfra, printer.Fail("No se pudo crear la ejecución de '%s': %s", run.Slug, err))
		}
		run.Outcome = ciOutcomeFailed
		run.ErrorMessage = fmt.Sprintf("no se pudo crear la ejecución: %s", err)
		return nil
	}
	run.ExecutionRefId = execution.RefId
	applyExecutionToRun(run, execution)
	return nil
}

func waitForCIRuns(
	client ciClient,
	token string,
	runs []*ciFlowRun,
	interval time.Duration,
	deadline time.Time,
	printer *ciPrinter,
) error {
	consecutiveErrors := 0
	for {
		pending := pendingCIRuns(runs)
		if len(pending) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return withExitCode(ciExitInfra, printer.Fail(
				"Timeout global alcanzado con %d ejecución(es) sin terminar; siguen corriendo en el backoffice",
				len(pending),
			))
		}
		for _, run := range pending {
			execution, err := client.GetExecution(token, run.ExecutionRefId)
			if err != nil {
				if !api.IsConnectionError(err) {
					return withExitCode(ciExitInfra, printer.Fail("No se pudo consultar la ejecución de '%s': %s", run.Slug, err))
				}
				consecutiveErrors++
				if consecutiveErrors >= ciMaxConsecutivePollErrors {
					return withExitCode(ciExitInfra, printer.Fail("Se perdió la conexión con el API tras %d intentos: %s", consecutiveErrors, err))
				}
				printer.Warning(fmt.Sprintf("Fallo temporal consultando '%s' (%d/%d); se reintenta", run.Slug, consecutiveErrors, ciMaxConsecutivePollErrors))
				break
			}
			consecutiveErrors = 0
			applyExecutionToRun(run, execution)
			if run.Outcome != ciOutcomePending {
				printer.Result(run)
			}
		}
		remaining := pendingCIRuns(runs)
		if len(remaining) == 0 {
			return nil
		}
		printer.Waiting(len(remaining))
		time.Sleep(interval)
	}
}

func pendingCIRuns(runs []*ciFlowRun) []*ciFlowRun {
	var pending []*ciFlowRun
	for _, run := range runs {
		if run.Outcome == ciOutcomePending && run.ExecutionRefId != "" {
			pending = append(pending, run)
		}
	}
	return pending
}

func markCISkippedFrom(runs []*ciFlowRun, start int, printer *ciPrinter) {
	for _, run := range runs[start:] {
		if run.Outcome != ciOutcomePending {
			continue
		}
		run.Outcome = ciOutcomeSkipped
		run.SkipReason = ciSkipReasonFailFast
		printer.Result(run)
	}
}

func applyExecutionToRun(run *ciFlowRun, execution *types.Execution) {
	run.Status = execution.Status
	run.MatchStatus = execution.MatchStatus
	run.ErrorMessage = execution.ErrorMessage
	run.DurationMs = execution.DurationMs
	if execution.FailureContext != nil {
		run.FailedStep = execution.FailureContext.FailedStepIndex
		run.FailureURL = execution.FailureContext.URL
		if run.ErrorMessage == "" {
			run.ErrorMessage = execution.FailureContext.Error
		}
	}
	if !isTerminalExecutionStatus(execution.Status) {
		return
	}
	if executionPassed(execution) {
		run.Outcome = ciOutcomePassed
		return
	}
	run.Outcome = ciOutcomeFailed
	if run.ErrorMessage == "" {
		run.ErrorMessage = fmt.Sprintf("la ejecución terminó en %s", ciFailLabel(run))
	}
}

func isTerminalExecutionStatus(status string) bool {
	switch status {
	case types.ExecutionStatusSuccess, types.ExecutionStatusFailed, types.ExecutionStatusCancelled:
		return true
	}
	return false
}

func executionPassed(execution *types.Execution) bool {
	return execution.Status == types.ExecutionStatusSuccess &&
		execution.MatchStatus != types.ExecutionMatchStatusMismatch
}

package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

const (
	ciVerdictPass    = "PASS"
	ciVerdictFail    = "FAIL"
	ciVerdictPending = "PENDING"
)

const ciResultMessageMaxLength = 160

type ciVerdict struct {
	Verdict  string      `json:"verdict"`
	Total    int         `json:"total"`
	Passed   int         `json:"passed"`
	Failed   int         `json:"failed"`
	Pending  int         `json:"pending,omitempty"`
	Skipped  []string    `json:"skipped"`
	Failures []ciFailure `json:"failures"`
}

type ciFailure struct {
	FlowSlug        string `json:"flowSlug"`
	FlowName        string `json:"flowName"`
	ExecutionRefId  string `json:"executionRefId,omitempty"`
	Status          string `json:"status,omitempty"`
	MatchStatus     string `json:"matchStatus,omitempty"`
	ErrorMessage    string `json:"errorMessage"`
	FailedStepIndex *int   `json:"failedStepIndex"`
	DurationMs      *int64 `json:"durationMs"`
}

func buildVerdict(runs []*ciFlowRun, paramSkipped []string) ciVerdict {
	verdict := ciVerdict{
		Verdict:  ciVerdictPass,
		Skipped:  append([]string{}, paramSkipped...),
		Failures: []ciFailure{},
	}
	for _, run := range runs {
		verdict.Total++
		switch run.Outcome {
		case ciOutcomePassed:
			verdict.Passed++
		case ciOutcomeFailed:
			verdict.Failed++
			verdict.Failures = append(verdict.Failures, ciFailureFrom(run))
		case ciOutcomeSkipped:
			verdict.Skipped = append(verdict.Skipped, run.Slug)
		case ciOutcomePending:
			verdict.Pending++
		}
	}
	if verdict.Failed > 0 {
		verdict.Verdict = ciVerdictFail
	} else if verdict.Pending > 0 {
		verdict.Verdict = ciVerdictPending
	}
	return verdict
}

func ciFailureFrom(run *ciFlowRun) ciFailure {
	return ciFailure{
		FlowSlug:        run.Slug,
		FlowName:        run.Name,
		ExecutionRefId:  run.ExecutionRefId,
		Status:          run.Status,
		MatchStatus:     run.MatchStatus,
		ErrorMessage:    run.ErrorMessage,
		FailedStepIndex: run.FailedStep,
		DurationMs:      run.DurationMs,
	}
}

func ciSummaryLabel(verdict ciVerdict) string {
	switch verdict.Verdict {
	case ciVerdictFail:
		label := fmt.Sprintf("%d de %d workflow(s) fallaron", verdict.Failed, verdict.Total)
		if skipped := verdict.Total - verdict.Passed - verdict.Failed - verdict.Pending; skipped > 0 {
			label = fmt.Sprintf("%s · %d omitido(s)", label, skipped)
		}
		return label
	case ciVerdictPending:
		return fmt.Sprintf("%d ejecución(es) encoladas; no se esperó el resultado (--wait=false)", verdict.Pending)
	default:
		return fmt.Sprintf("%d/%d workflow(s) en verde", verdict.Passed, verdict.Total)
	}
}

func emitCIReport(opts ciOpts, projectName string, runs []*ciFlowRun, verdict ciVerdict, printer *ciPrinter) error {
	if opts.Output == ciOutputText {
		return nil
	}
	var document []byte
	var err error
	switch opts.Output {
	case ciOutputJSON:
		document, err = renderCIJSON(verdict)
	case ciOutputJUnit:
		document, err = renderCIJUnit(projectName, runs)
	}
	if err != nil {
		return err
	}
	if opts.OutFile != "" {
		if err := os.WriteFile(opts.OutFile, document, 0644); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", opts.OutFile, err)
		}
		printer.Step(fmt.Sprintf("reporte %s escrito en %s", opts.Output, opts.OutFile))
		return nil
	}
	printer.StopSpinner()
	if _, err := os.Stdout.Write(document); err != nil {
		return fmt.Errorf("no se pudo escribir el reporte a stdout: %w", err)
	}
	return nil
}

func renderCIJSON(verdict ciVerdict) ([]byte, error) {
	document, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("no se pudo generar el reporte json: %w", err)
	}
	return append(document, '\n'), nil
}

type junitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

func renderCIJUnit(projectName string, runs []*ciFlowRun) ([]byte, error) {
	suite := junitTestSuite{Name: projectName}
	var totalMs int64
	for _, run := range runs {
		suite.Tests++
		testCase := junitTestCase{
			Name:      run.Slug,
			Classname: projectName,
			Time:      junitSeconds(run.DurationMs),
		}
		if run.DurationMs != nil {
			totalMs += *run.DurationMs
		}
		switch run.Outcome {
		case ciOutcomeFailed:
			suite.Failures++
			testCase.Failure = &junitFailure{
				Message: ciFirstLine(run.ErrorMessage),
				Type:    ciFailLabel(run),
				Body:    junitFailureBody(run),
			}
		case ciOutcomeSkipped:
			suite.Skipped++
			testCase.Skipped = &junitSkipped{Message: run.SkipReason}
		case ciOutcomePending:
			suite.Skipped++
			testCase.Skipped = &junitSkipped{Message: "sin resultado (--wait=false)"}
		}
		suite.Cases = append(suite.Cases, testCase)
	}
	suite.Time = junitSecondsValue(totalMs)

	document, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("no se pudo generar el reporte junit: %w", err)
	}
	output := append([]byte(xml.Header), document...)
	return append(output, '\n'), nil
}

func junitFailureBody(run *ciFlowRun) string {
	lines := []string{
		fmt.Sprintf("Flow: %s", run.Name),
		fmt.Sprintf("Estado: %s", ciFailLabel(run)),
	}
	if run.FailedStep != nil {
		lines = append(lines, fmt.Sprintf("Paso fallido: %d", *run.FailedStep))
	}
	if run.FailureURL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", run.FailureURL))
	}
	if run.ErrorMessage != "" {
		lines = append(lines, "", run.ErrorMessage)
	}
	return strings.Join(lines, "\n")
}

func junitSeconds(durationMs *int64) string {
	if durationMs == nil {
		return "0.000"
	}
	return junitSecondsValue(*durationMs)
}

func junitSecondsValue(durationMs int64) string {
	return fmt.Sprintf("%.3f", float64(durationMs)/1000)
}

func ciFailLabel(run *ciFlowRun) string {
	if run.Status == "" {
		return "ERROR"
	}
	if run.Status == types.ExecutionStatusSuccess && run.MatchStatus == types.ExecutionMatchStatusMismatch {
		return types.ExecutionMatchStatusMismatch
	}
	return run.Status
}

func ciProjectLabel(project string) string {
	if strings.TrimSpace(project) == "" {
		return "oraculo"
	}
	return project
}

func ciFirstLine(message string) string {
	if message == "" {
		return "la ejecución falló"
	}
	line := strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
	if line == "" {
		return "la ejecución falló"
	}
	return line
}

func ciShortMessage(message string) string {
	line := ciFirstLine(message)
	runes := []rune(line)
	if len(runes) <= ciResultMessageMaxLength {
		return line
	}
	return string(runes[:ciResultMessageMaxLength]) + "…"
}

func ciDurationLabel(durationMs *int64) string {
	if durationMs == nil {
		return ""
	}
	return fmt.Sprintf(" en %.1fs", float64(*durationMs)/1000)
}

type ciPrinter struct {
	enabled bool
	spinner *ui.Spinner
}

func newCIPrinter(enabled bool) *ciPrinter {
	return &ciPrinter{enabled: enabled}
}

func (p *ciPrinter) Header(project, tokenSource string) {
	if !p.enabled {
		return
	}
	ui.PrintHeader(
		"CI",
		fmt.Sprintf("Gate E2E · %s", ciProjectLabel(project)),
		"Corre los workflows del proyecto y falla si alguno rompe.",
	)
	ui.PrintHint(fmt.Sprintf("Auth: %s", tokenSource))
}

func (p *ciPrinter) Step(message string) {
	if !p.enabled {
		return
	}
	ui.PrintStep(message)
}

func (p *ciPrinter) Success(message string) {
	if !p.enabled {
		return
	}
	ui.PrintSuccess(message)
}

func (p *ciPrinter) Enqueued(run *ciFlowRun) {
	p.Step(fmt.Sprintf("'%s' · ejecución encolada", run.Slug))
}

func (p *ciPrinter) Result(run *ciFlowRun) {
	p.StopSpinner()
	switch run.Outcome {
	case ciOutcomePassed:
		p.Step(fmt.Sprintf("'%s' · %s%s", run.Slug, run.Status, ciDurationLabel(run.DurationMs)))
	case ciOutcomeFailed:
		ui.PrintCheckFail(fmt.Sprintf("'%s' · %s · %s", run.Slug, ciFailLabel(run), ciShortMessage(run.ErrorMessage)))
	case ciOutcomeSkipped:
		if p.enabled {
			ui.PrintHint(fmt.Sprintf("'%s' · omitido (%s)", run.Slug, run.SkipReason))
		}
	}
}

func (p *ciPrinter) Waiting(pending int) {
	if !p.enabled || p.spinner != nil {
		return
	}
	p.spinner = ui.NewSpinner(fmt.Sprintf("Esperando %d ejecución(es)... (poll cada %s)", pending, ciPollInterval))
	p.spinner.Start()
}

func (p *ciPrinter) Warning(message string) {
	p.StopSpinner()
	ui.PrintWarning(message)
}

func (p *ciPrinter) Fail(format string, args ...any) error {
	p.StopSpinner()
	return ui.Fail(format, args...)
}

func (p *ciPrinter) StopSpinner() {
	if p.spinner == nil {
		return
	}
	p.spinner.Stop()
	p.spinner = nil
}

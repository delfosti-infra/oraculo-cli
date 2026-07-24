package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "actualiza los archivos golden")

func ciReportRuns() []*ciFlowRun {
	return []*ciFlowRun{
		{
			Slug:           "login",
			Name:           "Login",
			ExecutionRefId: "exec-1",
			Outcome:        ciOutcomePassed,
			Status:         "SUCCESS",
			DurationMs:     int64Ptr(12345),
		},
		{
			Slug:           "checkout",
			Name:           "Checkout",
			ExecutionRefId: "exec-2",
			Outcome:        ciOutcomeFailed,
			Status:         "FAILED",
			ErrorMessage:   "TimeoutError: locator('#pay') not found\n    at step 3",
			FailedStep:     intPtr(3),
			FailureURL:     "https://demo.test/checkout",
			DurationMs:     int64Ptr(42000),
		},
		{
			Slug:       "signup",
			Name:       "Signup",
			Outcome:    ciOutcomeSkipped,
			SkipReason: ciSkipReasonFailFast,
		},
	}
}

func TestBuildVerdict(t *testing.T) {
	cases := []struct {
		name         string
		runs         []*ciFlowRun
		paramSkipped []string
		wantVerdict  string
		wantTotal    int
		wantPassed   int
		wantFailed   int
		wantPending  int
		wantSkipped  []string
	}{
		{
			name:         "mezcla con fallo",
			runs:         ciReportRuns(),
			paramSkipped: []string{"con-params"},
			wantVerdict:  ciVerdictFail,
			wantTotal:    3,
			wantPassed:   1,
			wantFailed:   1,
			wantSkipped:  []string{"con-params", "signup"},
		},
		{
			name: "todo en verde",
			runs: []*ciFlowRun{
				{Slug: "login", Outcome: ciOutcomePassed},
				{Slug: "signup", Outcome: ciOutcomePassed},
			},
			wantVerdict: ciVerdictPass,
			wantTotal:   2,
			wantPassed:  2,
			wantSkipped: []string{},
		},
		{
			name: "encolado sin esperar",
			runs: []*ciFlowRun{
				{Slug: "login", Outcome: ciOutcomePending, ExecutionRefId: "exec-1"},
			},
			wantVerdict: ciVerdictPending,
			wantTotal:   1,
			wantPending: 1,
			wantSkipped: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict := buildVerdict(c.runs, c.paramSkipped)
			if verdict.Verdict != c.wantVerdict {
				t.Errorf("verdict = %q, se esperaba %q", verdict.Verdict, c.wantVerdict)
			}
			if verdict.Total != c.wantTotal || verdict.Passed != c.wantPassed ||
				verdict.Failed != c.wantFailed || verdict.Pending != c.wantPending {
				t.Errorf("counts = %+v", verdict)
			}
			if strings.Join(verdict.Skipped, ",") != strings.Join(c.wantSkipped, ",") {
				t.Errorf("skipped = %v, se esperaba %v", verdict.Skipped, c.wantSkipped)
			}
			if len(verdict.Failures) != c.wantFailed {
				t.Errorf("failures = %d, se esperaba %d", len(verdict.Failures), c.wantFailed)
			}
		})
	}
}

func TestCISummaryLabel(t *testing.T) {
	cases := []struct {
		name    string
		verdict ciVerdict
		want    string
	}{
		{"verde", ciVerdict{Verdict: ciVerdictPass, Total: 3, Passed: 3}, "3/3 workflow(s) en verde"},
		{"con fallos", ciVerdict{Verdict: ciVerdictFail, Total: 3, Passed: 1, Failed: 1}, "1 de 3 workflow(s) fallaron · 1 omitido(s)"},
		{"pendiente", ciVerdict{Verdict: ciVerdictPending, Total: 2, Pending: 2}, "2 ejecución(es) encoladas"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ciSummaryLabel(c.verdict)
			if !strings.Contains(got, c.want) {
				t.Errorf("label = %q, debía contener %q", got, c.want)
			}
		})
	}
}

func TestRenderCIJSON(t *testing.T) {
	verdict := buildVerdict(ciReportRuns(), []string{"con-params"})
	document, err := renderCIJSON(verdict)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(document, &parsed); err != nil {
		t.Fatalf("el reporte no es JSON válido: %v", err)
	}

	if parsed["verdict"] != "FAIL" {
		t.Errorf("verdict = %v, se esperaba FAIL", parsed["verdict"])
	}
	if parsed["total"] != float64(3) || parsed["passed"] != float64(1) || parsed["failed"] != float64(1) {
		t.Errorf("counts inesperados: %v", parsed)
	}

	failures, ok := parsed["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %v, se esperaba 1 entrada", parsed["failures"])
	}
	failure := failures[0].(map[string]any)
	if failure["flowSlug"] != "checkout" {
		t.Errorf("flowSlug = %v", failure["flowSlug"])
	}
	if failure["status"] != "FAILED" {
		t.Errorf("status = %v", failure["status"])
	}
	if failure["failedStepIndex"] != float64(3) {
		t.Errorf("failedStepIndex = %v", failure["failedStepIndex"])
	}
	if failure["executionRefId"] != "exec-2" {
		t.Errorf("executionRefId = %v", failure["executionRefId"])
	}
	if !strings.Contains(failure["errorMessage"].(string), "TimeoutError") {
		t.Errorf("errorMessage = %v", failure["errorMessage"])
	}
}

func TestRenderCIJUnitGolden(t *testing.T) {
	document, err := renderCIJUnit("Demo", ciReportRuns())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	golden := filepath.Join("testdata", "ci_junit.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("no se pudo crear testdata: %v", err)
		}
		if err := os.WriteFile(golden, document, 0644); err != nil {
			t.Fatalf("no se pudo actualizar el golden: %v", err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("no se pudo leer el golden (corre 'go test -run TestRenderCIJUnitGolden -update ./cmd'): %v", err)
	}
	if !bytes.Equal(document, want) {
		t.Errorf("junit no coincide con el golden:\n--- got ---\n%s\n--- want ---\n%s", document, want)
	}
}

func TestRenderCIJUnitIsValidXML(t *testing.T) {
	document, err := renderCIJUnit("Demo <&> \"quotes\"", ciReportRuns())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var suite junitTestSuite
	if err := xml.Unmarshal(document, &suite); err != nil {
		t.Fatalf("el reporte no es XML válido: %v", err)
	}

	if suite.Tests != 3 || suite.Failures != 1 || suite.Skipped != 1 || suite.Errors != 0 {
		t.Errorf("attrs = tests %d failures %d skipped %d errors %d", suite.Tests, suite.Failures, suite.Skipped, suite.Errors)
	}
	if suite.Time != "54.345" {
		t.Errorf("time = %q, se esperaba 54.345", suite.Time)
	}
	if len(suite.Cases) != 3 {
		t.Fatalf("cases = %d", len(suite.Cases))
	}

	failed := suite.Cases[1]
	if failed.Failure == nil {
		t.Fatal("el caso 'checkout' debía tener <failure>")
	}
	if !strings.Contains(failed.Failure.Message, "TimeoutError") {
		t.Errorf("failure message = %q", failed.Failure.Message)
	}
	if !strings.Contains(failed.Failure.Body, "Paso fallido: 3") {
		t.Errorf("failure body = %q, debía incluir el paso fallido", failed.Failure.Body)
	}

	skipped := suite.Cases[2]
	if skipped.Skipped == nil || skipped.Skipped.Message != ciSkipReasonFailFast {
		t.Errorf("el caso 'signup' debía estar skipped por fail-fast: %+v", skipped.Skipped)
	}
}

func TestCIFailLabel(t *testing.T) {
	cases := []struct {
		name string
		run  ciFlowRun
		want string
	}{
		{"sin ejecución", ciFlowRun{}, "ERROR"},
		{"failed", ciFlowRun{Status: "FAILED"}, "FAILED"},
		{"mismatch", ciFlowRun{Status: "SUCCESS", MatchStatus: "MISMATCH"}, "MISMATCH"},
		{"cancelled", ciFlowRun{Status: "CANCELLED"}, "CANCELLED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ciFailLabel(&c.run); got != c.want {
				t.Errorf("ciFailLabel = %q, se esperaba %q", got, c.want)
			}
		})
	}
}

func TestCIShortMessage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"vacío", "", "la ejecución falló"},
		{"una línea", "TimeoutError", "TimeoutError"},
		{"multilínea corta primera línea", "boom\nstack trace", "boom"},
		{"larga se trunca", strings.Repeat("x", 200), strings.Repeat("x", ciResultMessageMaxLength) + "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ciShortMessage(c.in); got != c.want {
				t.Errorf("ciShortMessage = %q, se esperaba %q", got, c.want)
			}
		})
	}
}

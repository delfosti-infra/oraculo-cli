package types

const (
	ExecutionStatusPending   = "PENDING"
	ExecutionStatusClaimed   = "CLAIMED"
	ExecutionStatusRunning   = "RUNNING"
	ExecutionStatusSuccess   = "SUCCESS"
	ExecutionStatusFailed    = "FAILED"
	ExecutionStatusCancelled = "CANCELLED"
)

const ExecutionMatchStatusMismatch = "MISMATCH"

type CreateExecutionRequest struct {
	ParamValues map[string]string `json:"paramValues"`
	TriggerNote string            `json:"triggerNote,omitempty"`
}

type ExecutionFailureContext struct {
	URL             string `json:"url"`
	Error           string `json:"error"`
	AriaSnapshot    string `json:"ariaSnapshot"`
	FailedStepIndex *int   `json:"failedStepIndex"`
}

type Execution struct {
	RefId          string                   `json:"refId"`
	FlowRefId      string                   `json:"flowRefId"`
	FlowSlug       string                   `json:"flowSlug"`
	FlowName       string                   `json:"flowName"`
	Status         string                   `json:"status"`
	MatchStatus    string                   `json:"matchStatus"`
	ErrorMessage   string                   `json:"errorMessage"`
	StartedAt      string                   `json:"startedAt"`
	FinishedAt     string                   `json:"finishedAt"`
	DurationMs     *int64                   `json:"durationMs"`
	SpecVersion    *int                     `json:"specVersion"`
	FailureContext *ExecutionFailureContext `json:"failureContext"`
}

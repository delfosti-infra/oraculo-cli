package types

type ToggleFlowCoreRequest struct {
	IsCore bool `json:"isCore"`
}

type FlowRef struct {
	RefId string `json:"refId"`
}

type FlowSummary struct {
	RefId       string `json:"refId"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	SpecContent string `json:"specContent"`
	SpecVersion int    `json:"specVersion"`
	UpdatedAt   string `json:"updatedAt"`
}

type UpdateFlowSpecRequest struct {
	SpecContent string `json:"specContent"`
}

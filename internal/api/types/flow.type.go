package types

type ToggleFlowCoreRequest struct {
	IsCore bool `json:"isCore"`
}

type FlowRef struct {
	RefId string `json:"refId"`
}

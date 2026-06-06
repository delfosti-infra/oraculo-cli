package types

type Project struct {
	RefId   string `json:"refId"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
}

type PageableResponse[T any] struct {
	Content []T `json:"content"`
}

package types

type Config struct {
	Project string `json:"project"`
	RefId   string `json:"refId"`
	BaseURL string `json:"base_url"`
	APIURL  string `json:"api_url"`
	E2EDir  string `json:"e2e_dir"`
}

package types

import (
	"encoding/json"
	"fmt"
)

type ApiResponse[T any] struct {
	Succeeded bool     `json:"succeeded"`
	Data      *T       `json:"data"`
	Message   string   `json:"message"`
	Errors    []string `json:"errors"`
	Path      string   `json:"path"`
}

func UnwrapJSON[T any](respBody []byte) (*T, error) {
	var wrapper ApiResponse[T]
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("no se pudo parsear la respuesta: %w", err)
	}
	if !wrapper.Succeeded {
		if wrapper.Message != "" {
			return nil, fmt.Errorf("%s", wrapper.Message)
		}
		return nil, fmt.Errorf("la API devolvió succeeded=false")
	}
	if wrapper.Data == nil {
		return nil, fmt.Errorf("la API devolvió data nula")
	}
	return wrapper.Data, nil
}

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/delfosti/oraculo-cli/internal/api/types"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Login(email, password string) (*types.AuthSession, error) {
	body, err := json.Marshal(types.LoginRequest{Email: email, Password: password})
	if err != nil {
		return nil, fmt.Errorf("no se pudo serializar la solicitud: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/auth/login",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar con el API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp types.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("%s", errResp.Message)
		}
		return nil, fmt.Errorf("login falló con código %d", resp.StatusCode)
	}

	var session types.AuthSession
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, fmt.Errorf("no se pudo parsear la respuesta: %w", err)
	}

	return &session, nil
}

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
		return nil, fmt.Errorf("%s", ErrorMessage(respBody, resp.StatusCode))
	}

	session, err := types.UnwrapJSON[types.AuthSession](respBody)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	return session, nil
}

func (c *Client) ListProjects(token string) ([]types.Project, error) {
	url := fmt.Sprintf("%s/projects?pageNumber=0&pageSize=200", c.baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo construir el request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar con el API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", ErrorMessage(respBody, resp.StatusCode))
	}

	page, err := types.UnwrapJSON[types.PageableResponse[types.Project]](respBody)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return page.Content, nil
}

func (c *Client) ToggleFlowCore(token, projectRefId, flowRefId string, isCore bool) error {
	body, err := json.Marshal(types.ToggleFlowCoreRequest{IsCore: isCore})
	if err != nil {
		return fmt.Errorf("no se pudo serializar la solicitud: %w", err)
	}

	url := fmt.Sprintf("%s/projects/%s/flows/%s/core", c.baseURL, projectRefId, flowRefId)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("no se pudo construir el request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo conectar con el API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("no se pudo leer la respuesta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", ErrorMessage(respBody, resp.StatusCode))
	}

	return nil
}

func ErrorMessage(respBody []byte, statusCode int) string {
	var env types.ErrorResponse
	if err := json.Unmarshal(respBody, &env); err == nil && env.Message != "" {
		return env.Message
	}
	body := string(respBody)
	if len(body) > 200 {
		body = body[:200]
	}
	return fmt.Sprintf("status %d: %s", statusCode, body)
}

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
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

// GetAuthSessionState descarga el storageState (Playwright) descifrado del proyecto.
// Devuelve (nil, nil) si no hay sesión usable: 404 (no existe) o 410 (expiró).
func (c *Client) GetAuthSessionState(token, projectRefId string) ([]byte, error) {
	url := fmt.Sprintf("%s/projects/%s/auth-session/state", c.baseURL, projectRefId)
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

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", ErrorMessage(respBody, resp.StatusCode))
	}

	raw, err := types.UnwrapJSON[json.RawMessage](respBody)
	if err != nil {
		return nil, fmt.Errorf("auth session state: %w", err)
	}
	return []byte(*raw), nil
}

// GetAuthSessionStatus devuelve la metadata de la sesión guardada (sin el secreto).
// Devuelve (nil, nil) si el proyecto no tiene sesión.
func (c *Client) GetAuthSessionStatus(token, projectRefId string) (*types.AuthSessionMetadata, error) {
	url := fmt.Sprintf("%s/projects/%s/auth-session", c.baseURL, projectRefId)
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", ErrorMessage(respBody, resp.StatusCode))
	}

	return types.UnwrapJSON[types.AuthSessionMetadata](respBody)
}

// PutAuthSession guarda (upsert) el storageState del proyecto, cifrado en el core.
func (c *Client) PutAuthSession(
	token, projectRefId string,
	storageState json.RawMessage,
	label string,
	expiresInDays int,
) (*types.AuthSessionMetadata, error) {
	payload := types.UpsertAuthSessionRequest{
		StorageState:  storageState,
		Label:         label,
		ExpiresInDays: expiresInDays,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("no se pudo serializar la solicitud: %w", err)
	}

	url := fmt.Sprintf("%s/projects/%s/auth-session", c.baseURL, projectRefId)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("no se pudo construir el request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
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

	return types.UnwrapJSON[types.AuthSessionMetadata](respBody)
}

// DeleteAuthSession borra la sesión guardada del proyecto.
func (c *Client) DeleteAuthSession(token, projectRefId string) error {
	url := fmt.Sprintf("%s/projects/%s/auth-session", c.baseURL, projectRefId)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("no se pudo construir el request: %w", err)
	}
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

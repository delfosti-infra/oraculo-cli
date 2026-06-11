// Package config centraliza el estado global de Oráculo en ~/.oraculo (token de
// sesión y backend por defecto) y resuelve contra qué API apuntar la CLI.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultAPIURL es el backend hosteado de Oráculo al que apunta la CLI cuando no
// hay flag, env var, config de proyecto ni sesión previa. Quienes desarrollan el
// core pueden apuntar a otro con --api-url o la variable ORACULO_API_URL.
const DefaultAPIURL = "https://dft-dev-useu2-oraculo-ca-001.jollymeadow-d0b48f68.eastus2.azurecontainerapps.io"

// APIURLEnvVar sobreescribe el backend sin tocar archivos (útil en CI o para devs).
const APIURLEnvVar = "ORACULO_API_URL"

// Global es el estado en ~/.oraculo/config.json: el API contra el cual te
// logueaste por última vez, para que init/switch/record lo reusen sin reconfigurar.
type Global struct {
	APIURL string `json:"api_url"`
}

func oraculoDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}
	return filepath.Join(home, ".oraculo"), nil
}

func ensureOraculoDir() (string, error) {
	dir, err := oraculoDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("no se pudo crear ~/.oraculo: %w", err)
	}
	return dir, nil
}

// LoadGlobal lee ~/.oraculo/config.json. Si no existe, devuelve un Global vacío.
func LoadGlobal() (*Global, error) {
	dir, err := oraculoDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if os.IsNotExist(err) {
		return &Global{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer ~/.oraculo/config.json: %w", err)
	}
	var g Global
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("~/.oraculo/config.json inválido: %w", err)
	}
	return &g, nil
}

// SaveGlobalAPIURL persiste el backend por defecto del usuario tras un login exitoso.
func SaveGlobalAPIURL(apiURL string) error {
	dir, err := ensureOraculoDir()
	if err != nil {
		return err
	}
	g, err := LoadGlobal()
	if err != nil {
		g = &Global{}
	}
	g.APIURL = NormalizeURL(apiURL)

	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("no se pudo serializar la config global: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		return fmt.Errorf("no se pudo guardar ~/.oraculo/config.json: %w", err)
	}
	return nil
}

// SaveToken guarda el JWT de sesión en ~/.oraculo/token (0600).
func SaveToken(token string) error {
	dir, err := ensureOraculoDir()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0600); err != nil {
		return fmt.Errorf("no se pudo guardar el token: %w", err)
	}
	return nil
}

// LoadToken lee el JWT de sesión guardado por 'oraculo login'.
func LoadToken() (string, error) {
	dir, err := oraculoDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		return "", fmt.Errorf("no se encontró token en ~/.oraculo/token. Corre 'oraculo login' primero")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token vacío. Corre 'oraculo login' de nuevo")
	}
	return token, nil
}

// NormalizeURL recorta espacios y el slash final para concatenar paths uniforme.
func NormalizeURL(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}

// ResolveAPIURL elige el backend en orden de prioridad: el flag --api-url, la env
// var ORACULO_API_URL, el api_url del proyecto (oraculo.config.json), el último
// API logueado (~/.oraculo/config.json) y, por último, el default hosteado.
func ResolveAPIURL(flag, projectAPIURL string) string {
	if flag != "" {
		return NormalizeURL(flag)
	}
	if env := strings.TrimSpace(os.Getenv(APIURLEnvVar)); env != "" {
		return NormalizeURL(env)
	}
	if projectAPIURL != "" {
		return NormalizeURL(projectAPIURL)
	}
	if g, err := LoadGlobal(); err == nil && g.APIURL != "" {
		return NormalizeURL(g.APIURL)
	}
	return DefaultAPIURL
}

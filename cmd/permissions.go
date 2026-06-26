package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

type geolocation struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

const geolocationPermission = "geolocation"

var allowedBrowserPermissions = map[string]bool{
	"geolocation":     true,
	"notifications":   true,
	"camera":          true,
	"microphone":      true,
	"clipboard-read":  true,
	"clipboard-write": true,
}

func allowedBrowserPermissionList() []string {
	out := make([]string, 0, len(allowedBrowserPermissions))
	for p := range allowedBrowserPermissions {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func validateBrowserPermissions(raw []string) (valid []string, invalid []string) {
	seen := make(map[string]bool)
	for _, item := range raw {
		for _, p := range strings.Split(item, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			if p == "" {
				continue
			}
			if !allowedBrowserPermissions[p] {
				invalid = append(invalid, p)
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			valid = append(valid, p)
		}
	}
	return valid, invalid
}

func parseGeolocation(raw string) (*geolocation, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("geolocalización inválida '%s': usa el formato lat,lng (ej. -12.0464,-77.0428)", raw)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("latitud inválida '%s': %w", strings.TrimSpace(parts[0]), err)
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("longitud inválida '%s': %w", strings.TrimSpace(parts[1]), err)
	}
	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("latitud fuera de rango (%g): debe estar entre -90 y 90", lat)
	}
	if lng < -180 || lng > 180 {
		return nil, fmt.Errorf("longitud fuera de rango (%g): debe estar entre -180 y 180", lng)
	}
	return &geolocation{Lat: lat, Lng: lng}, nil
}

func resolveBrowserPermissions(permFlag []string, geoFlag string) ([]string, *geolocation, error) {
	perms, invalid := validateBrowserPermissions(permFlag)
	if len(invalid) > 0 {
		ui.PrintWarning(fmt.Sprintf(
			"Permiso(s) no soportado(s) descartado(s): %s. Válidos: %s",
			strings.Join(invalid, ", "),
			strings.Join(allowedBrowserPermissionList(), ", "),
		))
	}

	geo, err := parseGeolocation(geoFlag)
	if err != nil {
		return nil, nil, err
	}

	hasGeoPerm := false
	for _, p := range perms {
		if p == geolocationPermission {
			hasGeoPerm = true
			break
		}
	}
	if geo != nil && !hasGeoPerm {
		perms = append(perms, geolocationPermission)
		hasGeoPerm = true
	}
	if hasGeoPerm && geo == nil {
		ui.PrintWarning("Permiso 'geolocation' sin coordenadas: pásalas con --geolocation lat,lng o defínelas en el proyecto (Ajustes › Permisos). El replay puede fallar si el flujo depende de la ubicación.")
	}

	if len(perms) > 0 {
		ui.PrintStep(fmt.Sprintf("Permisos del flujo: %s", strings.Join(perms, ", ")))
	}
	return perms, geo, nil
}

func renderPermissionsUseBlock(permissions []string, geo *geolocation) string {
	var block string
	if len(permissions) > 0 {
		quoted := make([]string, len(permissions))
		for i, p := range permissions {
			quoted[i] = fmt.Sprintf("%q", p)
		}
		block += fmt.Sprintf("\n    permissions: [%s],", strings.Join(quoted, ", "))
	}
	if geo != nil {
		block += fmt.Sprintf(
			"\n    geolocation: { latitude: %s, longitude: %s },",
			strconv.FormatFloat(geo.Lat, 'f', -1, 64),
			strconv.FormatFloat(geo.Lng, 'f', -1, 64),
		)
	}
	return block
}

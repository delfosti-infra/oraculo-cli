package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type geolocation struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

const (
	geolocationPermission = "geolocation"
	permMarker            = "__ORACULO_PERM__"
)

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

const permissionDetectionPrelude = `  page.on('console', (__m) => { const __t = __m.text(); if (__t.indexOf('` + permMarker + ` ') === 0) console.log(__t); });
  await page.addInitScript(() => {
    const mark = (p) => { try { console.log('` + permMarker + ` ' + p); } catch (e) {} };
    try {
      const g = navigator.geolocation;
      if (g && g.getCurrentPosition) { const o = g.getCurrentPosition.bind(g); g.getCurrentPosition = (s, e, opt) => { mark('geolocation'); return o(s, e, opt); }; }
      if (g && g.watchPosition) { const w = g.watchPosition.bind(g); g.watchPosition = (s, e, opt) => { mark('geolocation'); return w(s, e, opt); }; }
    } catch (e) {}
    try {
      const md = navigator.mediaDevices;
      if (md && md.getUserMedia) { const o = md.getUserMedia.bind(md); md.getUserMedia = (c) => { if (c && c.video) mark('camera'); if (c && c.audio) mark('microphone'); return o(c); }; }
    } catch (e) {}
    try {
      if (window.Notification && Notification.requestPermission) { const o = Notification.requestPermission.bind(Notification); Notification.requestPermission = (cb) => { mark('notifications'); return o(cb); }; }
    } catch (e) {}
    try {
      const c = navigator.clipboard;
      if (c && c.readText) { const o = c.readText.bind(c); c.readText = () => { mark('clipboard-read'); return o(); }; }
      if (c && c.writeText) { const o = c.writeText.bind(c); c.writeText = (t) => { mark('clipboard-write'); return o(t); }; }
    } catch (e) {}
  });
`

var testBodyOpenRegex = regexp.MustCompile(`async\s*\(\s*\{[^)]*\}\s*\)\s*=>\s*\{`)

func injectPermissionDetection(spec string) string {
	loc := testBodyOpenRegex.FindStringIndex(spec)
	if loc == nil {
		return spec
	}
	insertAt := loc[1]
	return spec[:insertAt] + "\n" + permissionDetectionPrelude + spec[insertAt:]
}

func parseDetectedPermissions(output string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, permMarker+" ")
		if idx < 0 {
			continue
		}
		if strings.ContainsAny(line, "'\"(){}+;`") {
			continue
		}
		fields := strings.Fields(line[idx+len(permMarker):])
		if len(fields) == 0 {
			continue
		}
		perm := strings.ToLower(fields[0])
		if !allowedBrowserPermissions[perm] || seen[perm] {
			continue
		}
		seen[perm] = true
		out = append(out, perm)
	}
	sort.Strings(out)
	return out
}

func sliceContains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func resolveGeolocationByIP() (*geolocation, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipapi.co/json/")
	if err != nil {
		return nil, fmt.Errorf("geo-IP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("geo-IP devolvió status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("geo-IP: %w", err)
	}
	var data struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("geo-IP: respuesta inválida: %w", err)
	}
	if data.Latitude == nil || data.Longitude == nil {
		return nil, fmt.Errorf("geo-IP: sin coordenadas en la respuesta")
	}
	return &geolocation{Lat: *data.Latitude, Lng: *data.Longitude}, nil
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

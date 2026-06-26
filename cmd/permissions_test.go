package cmd

import (
	"reflect"
	"testing"
)

func TestValidateBrowserPermissions(t *testing.T) {
	cases := []struct {
		name        string
		in          []string
		wantValid   []string
		wantInvalid []string
	}{
		{
			name:      "csv y case-insensitive",
			in:        []string{"Geolocation, CAMERA"},
			wantValid: []string{"geolocation", "camera"},
		},
		{
			name:        "descarta no soportados",
			in:          []string{"geolocation", "midi"},
			wantValid:   []string{"geolocation"},
			wantInvalid: []string{"midi"},
		},
		{
			name:      "dedupe",
			in:        []string{"camera", "camera"},
			wantValid: []string{"camera"},
		},
		{
			name: "vacio",
			in:   []string{"", "  "},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			valid, invalid := validateBrowserPermissions(tc.in)
			if !reflect.DeepEqual(valid, tc.wantValid) {
				t.Errorf("valid = %v, want %v", valid, tc.wantValid)
			}
			if !reflect.DeepEqual(invalid, tc.wantInvalid) {
				t.Errorf("invalid = %v, want %v", invalid, tc.wantInvalid)
			}
		})
	}
}

func TestParseGeolocation(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    *geolocation
		wantErr bool
	}{
		{name: "vacio devuelve nil", in: ""},
		{name: "valido", in: "-12.0464,-77.0428", want: &geolocation{Lat: -12.0464, Lng: -77.0428}},
		{name: "con espacios", in: " 40.4, -3.7 ", want: &geolocation{Lat: 40.4, Lng: -3.7}},
		{name: "formato malo", in: "12.0", wantErr: true},
		{name: "no numerico", in: "abc,def", wantErr: true},
		{name: "lat fuera de rango", in: "91,0", wantErr: true},
		{name: "lng fuera de rango", in: "0,181", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGeolocation(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("se esperaba error para %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got = %+v, want %+v", got, tc.want)
			}
		})
	}
}

package types

import "encoding/json"

// UpsertAuthSessionRequest es el body para guardar el storageState de un proyecto.
type UpsertAuthSessionRequest struct {
	StorageState  json.RawMessage `json:"storageState"`
	Label         string          `json:"label,omitempty"`
	ExpiresInDays int             `json:"expiresInDays,omitempty"`
}

// AuthSessionMetadata es la metadata de la sesión (sin el secreto).
type AuthSessionMetadata struct {
	RefId      string  `json:"refId"`
	Label      string  `json:"label"`
	CapturedAt string  `json:"capturedAt"`
	ExpiresAt  *string `json:"expiresAt"`
	Expired    bool    `json:"expired"`
}

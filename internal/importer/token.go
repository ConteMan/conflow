package importer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrTokenExpired is returned when the preview token has expired or is invalid.
	ErrTokenExpired = errors.New("preview token expired")
	// ErrRevisionChanged is returned when the draft revision has changed since
	// the token was generated.
	ErrRevisionChanged = errors.New("draft revision changed since preview")
)

const tokenTTL = 15 * time.Minute

type tokenPayload struct {
	Revision  uint64    `json:"revision"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GenerateToken creates a preview token that encodes the current draft revision
// and an expiry time 15 minutes in the future.
func GenerateToken(draftRevision uint64) string {
	payload := tokenPayload{
		Revision:  draftRevision,
		ExpiresAt: time.Now().UTC().Add(tokenTTL),
	}
	data, _ := json.Marshal(payload)
	return base64.URLEncoding.EncodeToString(data)
}

// ValidateToken checks that the token is syntactically valid, not expired, and
// that the draft revision matches the one encoded in the token.
// Returns ErrTokenExpired for invalid or expired tokens, ErrRevisionChanged if
// the revision has advanced since the token was issued.
func ValidateToken(token string, currentRevision uint64) error {
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return ErrTokenExpired
	}
	var payload tokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ErrTokenExpired
	}
	if time.Now().UTC().After(payload.ExpiresAt) {
		return ErrTokenExpired
	}
	if payload.Revision != currentRevision {
		return ErrRevisionChanged
	}
	return nil
}

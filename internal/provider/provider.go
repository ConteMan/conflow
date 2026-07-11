// Package provider contains target-platform adapters. It never owns Pack
// rules, plan lifecycle, or HTTP presentation.
package provider

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("provider is not configured")
	ErrUnauthorized  = errors.New("provider unauthorized")
	ErrUnavailable   = errors.New("provider unavailable")
	ErrValidation    = errors.New("provider validation failed")
	ErrETagMismatch  = errors.New("provider etag mismatch")
)

type Capabilities struct {
	Pull     bool `json:"pull"`
	Validate bool `json:"validate"`
	Publish  bool `json:"publish"`
	Rollback bool `json:"rollback"`
}

type Status struct {
	Status       string
	Capabilities Capabilities
}

type Template struct {
	Raw        []byte
	ETag       string
	Version    string
	ObservedAt time.Time
}

// Adapter owns only provider protocol calls. Credentials are represented by a
// local reference and are never returned by this interface.
type Adapter interface {
	Connect(context.Context) error
	Status(context.Context) Status
	Pull(context.Context) (Template, error)
	Validate(context.Context, []byte) error
	Capabilities() Capabilities
}

// Publisher is deliberately separate from Adapter so read-only test doubles
// and future providers do not accidentally advertise a destructive capability.
type Publisher interface {
	Adapter
	Publish(context.Context, []byte, string) (Template, error)
}

func SafeError(err error) error {
	if err == nil {
		return nil
	}
	text := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrNotConfigured), errors.Is(err, ErrUnauthorized), errors.Is(err, ErrUnavailable), errors.Is(err, ErrValidation), errors.Is(err, ErrETagMismatch):
		return err
	case strings.Contains(text, "unauthorized"), strings.Contains(text, "forbidden"):
		return ErrUnauthorized
	default:
		return ErrUnavailable
	}
}

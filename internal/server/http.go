package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ConteMan/conflow/internal/project"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func apiMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := newRequestID()
		writer.Header().Set("X-Request-ID", requestID)
		writer.Header().Set("Cache-Control", "no-store")
		request = request.WithContext(withRequestID(request.Context(), requestID))

		if !allowedHost(request.Host) {
			writeAPIError(writer, request, http.StatusBadRequest, "invalid_host", "请求 Host 不允许", 0)
			return
		}
		if !allowedOrigin(request) {
			writeAPIError(writer, request, http.StatusForbidden, "invalid_origin", "请求 Origin 不允许", 0)
			return
		}
		if requiresJSON(request.Method) && !hasJSONContentType(request.Header.Get("Content-Type")) {
			writeAPIError(writer, request, http.StatusUnsupportedMediaType, "unsupported_media_type", "请求必须使用 application/json", 0)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func newRequestID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err == nil {
		return "req_" + hex.EncodeToString(value)
	}
	return fmt.Sprintf("req_%x", time.Now().UnixNano())
}

func requestID(request *http.Request) string {
	value, _ := request.Context().Value(requestIDKey).(string)
	return value
}

func allowedHost(rawHost string) bool {
	host := rawHost
	if parsed, _, err := net.SplitHostPort(rawHost); err == nil {
		host = parsed
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost")
}

func allowedOrigin(request *http.Request) bool {
	if request.Method == http.MethodGet || request.Method == http.MethodHead || request.Method == http.MethodOptions {
		return true
	}
	rawOrigin := request.Header.Get("Origin")
	if rawOrigin == "" {
		return true
	}
	origin, err := url.Parse(rawOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
		return false
	}
	return strings.EqualFold(origin.Host, request.Host)
}

func requiresJSON(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

func hasJSONContentType(raw string) bool {
	mediaType, _, err := mime.ParseMediaType(raw)
	return err == nil && mediaType == "application/json"
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func requireRevision(writer http.ResponseWriter, request *http.Request) (uint64, bool) {
	raw := request.Header.Get("If-Match")
	if raw == "" {
		writeAPIError(writer, request, http.StatusPreconditionRequired, "precondition_required", "修改请求必须提供 If-Match", 0)
		return 0, false
	}
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "If-Match 必须是带引号的 revision", 0)
		return 0, false
	}
	revision, err := strconv.ParseUint(raw[1:len(raw)-1], 10, 64)
	if err != nil || revision == 0 {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "If-Match revision 不合法", 0)
		return 0, false
	}
	return revision, true
}

func writeSuccess(writer http.ResponseWriter, request *http.Request, status int, data any, revision uint64) {
	writer.Header().Set("ETag", strconv.Quote(strconv.FormatUint(revision, 10)))
	writeJSON(writer, status, responseEnvelope{
		Data: data,
		Meta: responseMeta{RequestID: requestID(request), Revision: revision},
	})
}

func writeRequestError(writer http.ResponseWriter, request *http.Request, _ error) {
	writeAPIError(writer, request, http.StatusBadRequest, "malformed_json", "请求 JSON 不合法或包含未知字段", 0)
}

func writeAPIError(writer http.ResponseWriter, request *http.Request, status int, code, message string, currentRevision uint64) {
	writeJSON(writer, status, errorEnvelope{Error: errorDTO{
		Code:            code,
		Message:         message,
		RequestID:       requestID(request),
		CurrentRevision: currentRevision,
	}})
}

func writeManifestRevisionMismatch(writer http.ResponseWriter, request *http.Request, snapshot project.Snapshot) {
	writer.Header().Set("ETag", strconv.Quote(strconv.FormatUint(snapshot.Revision, 10)))
	writeJSON(writer, http.StatusPreconditionFailed, manifestRevisionMismatchEnvelope{Error: manifestRevisionMismatchDTO{
		Code:            "revision_mismatch",
		Message:         "项目已被其他操作修改，请重新加载",
		RequestID:       requestID(request),
		CurrentRevision: snapshot.Revision,
		CurrentState: manifestStateDTO{
			Project:      projectDTOFrom(snapshot.Manifest),
			Environments: environmentsDTOFrom(snapshot.Manifest.Environments),
		},
	}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

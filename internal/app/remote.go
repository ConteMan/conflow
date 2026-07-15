package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/remote"
)

var ErrRemoteSnapshotUnavailable = errors.New("remote snapshot unavailable")

type RemoteValueProjection struct {
	ProjectionID string `json:"projection_id"`
	EntityRef    string `json:"entity_ref"`
	FieldPath    string `json:"field_path"`
	ParameterKey string `json:"parameter_key"`
	ValueSummary string `json:"value_summary"`
	SnapshotETag string `json:"snapshot_etag"`
	ObservedAt   string `json:"observed_at"`
	Availability string `json:"availability"`
	Redacted     bool   `json:"redacted"`
	ReasonCode   string `json:"reason_code,omitempty"`
}
type RemoteProjection struct {
	EnvironmentID string                  `json:"environment_id"`
	SnapshotETag  string                  `json:"snapshot_etag"`
	Version       string                  `json:"version"`
	ObservedAt    string                  `json:"observed_at"`
	Projections   []RemoteValueProjection `json:"projections"`
}

func (s *Service) RemoteProjection(ctx context.Context, environmentID string) (RemoteProjection, error) {
	view, _, err := s.GetDraft(ctx, environmentID)
	if err != nil {
		return RemoteProjection{}, err
	}
	snapshot, err := s.remote.Current(environmentID)
	if err != nil {
		return RemoteProjection{}, err
	}
	if snapshot.Status != "available" {
		return RemoteProjection{}, ErrRemoteSnapshotUnavailable
	}
	result := RemoteProjection{EnvironmentID: environmentID, SnapshotETag: snapshot.RemoteETag, Version: snapshot.Version, ObservedAt: snapshot.ObservedAt.UTC().Format("2006-01-02T15:04:05Z"), Projections: []RemoteValueProjection{}}
	for collection, entityType := range map[string]string{"frequency_policies": "frequency_policy", "feature_switches": "feature_switch", "placements": "placement", "unit_bindings": "unit_binding"} {
		for _, record := range entities.Records(view.Effective, collection) {
			fields := make([]string, 0, len(record.Fields))
			for field := range record.Fields {
				fields = append(fields, field)
			}
			sort.Strings(fields)
			for _, field := range fields {
				key := remoteParameterKey(entityType, record.ID, field)
				projection := RemoteValueProjection{ProjectionID: projectionID(environmentID, key), EntityRef: "entity:" + view.PackRef + ":" + entityType + ":" + record.ID, FieldPath: "/" + field, ParameterKey: key, SnapshotETag: snapshot.RemoteETag, ObservedAt: result.ObservedAt, Availability: "unmapped", Redacted: false, ReasonCode: "parameter_unmapped"}
				if entityType == "unit_binding" || field == "unit_id_ref" {
					projection.Availability = "redacted"
					projection.Redacted = true
					projection.ReasonCode = "sensitive_value"
					projection.ValueSummary = "[redacted]"
				} else if value, ok := snapshot.Parameters[key]; ok {
					projection.Availability = "available"
					projection.ReasonCode = ""
					projection.ValueSummary = remoteSummary(value)
				}
				result.Projections = append(result.Projections, projection)
			}
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ProjectionID < result.Projections[j].ProjectionID })
	return result, nil
}

func remoteParameterKey(typ, id, field string) string {
	if typ == "frequency_policy" && field == "cooldown_ms" {
		return "ad_frequency_" + id
	}
	return typ + "_" + id + "_" + field
}
func projectionID(environmentID, key string) string {
	sum := sha256.Sum256([]byte(environmentID + "|" + key))
	return "rvp_" + hex.EncodeToString(sum[:])[:16]
}
func remoteSummary(value any) string {
	if text, ok := value.(string); ok {
		if milliseconds, err := strconv.Atoi(text); err == nil && milliseconds%1000 == 0 {
			return strconv.Itoa(milliseconds/1000) + " seconds"
		}
		return text
	}
	b, _ := json.Marshal(value)
	return string(b)
}

// inspectRemote is intentionally private provider-neutral comparison metadata.
// It records condition/structure observations for Plan risk analysis without
// placing Firebase values into any public response.
func inspectRemote(snapshot *remote.Snapshot, desired map[string]any) {
	if snapshot == nil || snapshot.Summary == nil {
		return
	}
	expected := map[string]bool{}
	for collection, entityType := range map[string]string{"frequency_policies": "frequency_policy", "feature_switches": "feature_switch", "placements": "placement", "unit_bindings": "unit_binding"} {
		for _, record := range entities.Records(desired, collection) {
			for field := range record.Fields {
				expected[remoteParameterKey(entityType, record.ID, field)] = true
			}
		}
	}
	for key := range snapshot.Parameters {
		if !expected[key] {
			snapshot.Summary.HasUnknownParameters = true
		}
	}
}

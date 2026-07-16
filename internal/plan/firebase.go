package plan

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ConteMan/conflow/internal/entities"
)

// MergeFirebaseTemplate changes only managed default values selected by the
// immutable Plan. It keeps every unselected parameter, condition, and
// conditional value exactly as Firebase returned it.
func MergeFirebaseTemplate(remoteTemplate, desiredJSON []byte, changes []RemoteParameterChange, packRef, environmentID string) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(remoteTemplate, &document); err != nil {
		return nil, fmt.Errorf("parse remote template: %w", err)
	}
	parameters, ok := document["parameters"].(map[string]any)
	if !ok {
		parameters = map[string]any{}
		document["parameters"] = parameters
	}
	var desired map[string]any
	if err := json.Unmarshal(desiredJSON, &desired); err != nil {
		return nil, fmt.Errorf("parse provider input: %w", err)
	}
	values := desiredParameterValues(desired, packRef, environmentID)
	keys := make([]string, 0, len(changes))
	allV2Parameters := false
	for _, change := range changes {
		if packRef == "mobile-ad-monetization/v2" && change.ParameterKey == "remote_config_layout_changed" {
			allV2Parameters = true
			continue
		}
		keys = append(keys, change.ParameterKey)
	}
	if allV2Parameters {
		keys = keys[:0]
		for key := range values {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			if changeKind(changes, key) == "deleted" {
				delete(parameters, key)
				continue
			}
			return nil, fmt.Errorf("managed parameter %q has no desired value", key)
		}
		parameter, ok := parameters[key].(map[string]any)
		if !ok {
			parameter = map[string]any{}
			parameters[key] = parameter
		}
		parameter["defaultValue"] = map[string]any{"value": firebaseValue(value)}
		parameter["valueType"] = inferValueType(value)
	}
	merged, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode remote template: %w", err)
	}
	return merged, nil
}

func changeKind(changes []RemoteParameterChange, key string) string {
	for _, change := range changes {
		if change.ParameterKey == key {
			return change.ChangeKind
		}
	}
	return ""
}

func desiredParameterValues(desired map[string]any, packRef, environmentID string) map[string]any {
	if packRef == "mobile-ad-monetization/v2" {
		return compileV2Parameters(desired, environmentID)
	}
	values := map[string]any{}
	for collection, entityType := range map[string]string{"frequency_policies": "frequency_policy", "feature_switches": "feature_switch", "placements": "placement", "unit_bindings": "unit_binding"} {
		for _, record := range entities.Records(desired, collection) {
			for field, value := range record.Fields {
				values[parameterKey(entityType, record.ID, field)] = value
			}
		}
	}
	return values
}

func inferValueType(value any) string {
	switch v := value.(type) {
	case bool:
		return "BOOLEAN"
	case float64, float32:
		return "NUMBER"
	case string:
		t := strings.TrimSpace(v)
		if len(t) > 1 && ((t[0] == '{' && t[len(t)-1] == '}') || (t[0] == '[' && t[len(t)-1] == ']')) && json.Valid([]byte(t)) {
			return "JSON"
		}
		return "STRING"
	default:
		return "STRING"
	}
}

func firebaseValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	default:
		return fmt.Sprint(value)
	}
}

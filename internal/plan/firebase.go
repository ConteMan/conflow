package plan

import (
	"encoding/json"
	"fmt"
)

// MergeFirebaseTemplate changes only managed default values selected by the
// immutable Plan. It keeps every unselected parameter, condition, and
// conditional value exactly as Firebase returned it.
func MergeFirebaseTemplate(remoteTemplate, desiredJSON []byte, changes []RemoteParameterChange) ([]byte, error) {
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
	values := desiredParameterValues(desired)
	for _, change := range changes {
		value, exists := values[change.ParameterKey]
		if !exists {
			return nil, fmt.Errorf("managed parameter %q has no desired value", change.ParameterKey)
		}
		parameter, ok := parameters[change.ParameterKey].(map[string]any)
		if !ok {
			parameter = map[string]any{}
			parameters[change.ParameterKey] = parameter
		}
		parameter["defaultValue"] = map[string]any{"value": firebaseValue(value)}
	}
	merged, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode remote template: %w", err)
	}
	return merged, nil
}

func desiredParameterValues(desired map[string]any) map[string]any {
	values := map[string]any{}
	for collection, entityType := range map[string]string{"frequency_policies": "frequency_policy", "feature_switches": "feature_switch", "placements": "placement", "unit_bindings": "unit_binding"} {
		for _, raw := range asSlice(desired[collection]) {
			record, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id, _ := record["id"].(string)
			for field, value := range record {
				if field != "id" {
					values[parameterKey(entityType, id, field)] = value
				}
			}
		}
	}
	return values
}

func firebaseValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	case float32:
		return fmt.Sprintf("%.0f", value)
	default:
		return fmt.Sprint(value)
	}
}

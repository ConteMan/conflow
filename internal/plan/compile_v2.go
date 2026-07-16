package plan

import (
	"encoding/json"
	"sort"

	"github.com/ConteMan/conflow/internal/entities"
)

// compileV2Parameters reads the v2 effective config and returns the managed
// parameter map: paramKey -> Go value (bool, string, number, or JSON string).
func compileV2Parameters(desired map[string]any, environmentID string) map[string]any {
	values := map[string]any{}
	layout, found := records(desired["remote_config_layouts"])["default"]
	if !found {
		return values
	}

	for _, featureSwitch := range sortedRecords(desired, "feature_switches") {
		key, keyOK := featureSwitch.Fields["key"].(string)
		value, valueOK := featureSwitch.Fields["default_value"].(bool)
		if keyOK && key != "" && valueOK {
			values[key] = value
		}
	}

	for _, parameter := range sortedRecords(desired, "custom_parameters") {
		key, keyOK := parameter.Fields["key"].(string)
		valueType, typeOK := parameter.Fields["value_type"].(string)
		value, valueOK := parameter.Fields["value"]
		if !keyOK || key == "" || !typeOK || !valueOK {
			continue
		}
		if valueType == "json" {
			values[key] = marshalV2JSON(value)
			continue
		}
		values[key] = value
	}

	network, networkFound := records(desired["network_settings"])["default"]
	if networkFound {
		if key, ok := layout.Fields["active_network_parameter_key"].(string); ok && key != "" {
			if activeNetwork, ok := network.Fields["active_network"].(string); ok {
				values[key] = activeNetwork
			}
		}
		if key, ok := layout.Fields["mediation_strategy_parameter_key"].(string); ok && key != "" {
			strategy, _ := network.Fields["mediation_strategy"].(string)
			values[key] = strategy
		}
	}

	if key, ok := layout.Fields["frequency_policies_parameter_key"].(string); ok && key != "" {
		values[key] = marshalV2JSON(map[string]any{"version": 2, "policies": v2Policies(desired)})
	}
	if key, ok := layout.Fields["placements_parameter_key"].(string); ok && key != "" {
		values[key] = marshalV2JSON(map[string]any{"version": 2, "placements": v2Placements(desired, environmentID)})
	}
	return values
}

func v2Policies(desired map[string]any) map[string]any {
	policies := map[string]any{}
	for _, policy := range sortedRecords(desired, "frequency_policies") {
		policies[policy.ID] = map[string]any{
			"cooldown":    normalizedV2FrequencyValue(policy.Fields["cooldown"]),
			"interval":    normalizedV2FrequencyValue(policy.Fields["interval"]),
			"max_count":   normalizedV2FrequencyValue(policy.Fields["max_count"]),
			"shift_count": normalizedV2FrequencyValue(policy.Fields["shift_count"]),
			"positions":   normalizedPositions(policy.Fields["positions"]),
		}
	}
	return policies
}

func v2Placements(desired map[string]any, environmentID string) []any {
	featureSwitches := records(desired["feature_switches"])
	bindingsByPlacement := map[string][]entities.Record{}
	for _, binding := range sortedRecords(desired, "unit_bindings") {
		bindingEnvironmentID, _ := binding.Fields["environment_id"].(string)
		if bindingEnvironmentID != environmentID {
			continue
		}
		placementID, _ := binding.Fields["placement_id"].(string)
		bindingsByPlacement[placementID] = append(bindingsByPlacement[placementID], binding)
	}
	placements := sortedRecords(desired, "placements")
	sort.SliceStable(placements, func(i, j int) bool {
		left, _ := placements[i].Fields["client_id"].(string)
		right, _ := placements[j].Fields["client_id"].(string)
		if left == right {
			return placements[i].ID < placements[j].ID
		}
		return left < right
	})
	result := make([]any, 0, len(placements))
	for _, placement := range placements {
		bindings := bindingsByPlacement[placement.ID]
		sort.SliceStable(bindings, func(i, j int) bool {
			return v2BindingSortKey(bindings[i]) < v2BindingSortKey(bindings[j])
		})
		units := map[string]any{}
		for _, binding := range bindings {
			network, _ := binding.Fields["network"].(string)
			if network == "" {
				continue
			}
			if _, exists := units[network]; exists {
				continue
			}
			units[network] = map[string]any{"unit_id": binding.Fields["unit_id_ref"]}
		}
		enabledConfigKey := ""
		enabledSwitchID, _ := placement.Fields["enabled_switch_id"].(string)
		if featureSwitch, found := featureSwitches[enabledSwitchID]; found {
			enabledConfigKey, _ = featureSwitch.Fields["key"].(string)
		}
		result = append(result, map[string]any{
			"id":                      placement.Fields["client_id"],
			"placement":               placement.Fields["key"],
			"type":                    placement.Fields["ad_type"],
			"enabled_config_key":      enabledConfigKey,
			"network_mode":            placement.Fields["network_mode"],
			"units":                   units,
			"frequency_policy_type":   placement.Fields["frequency_policy_type"],
			"frequency_policy_id":     placement.Fields["frequency_policy_id"],
			"custom_frequency_policy": normalizedV2FrequencyValue(placement.Fields["custom_frequency_policy"]),
			"load_timeout_ms":         placement.Fields["load_timeout_ms"],
			"cache_ttl_seconds":       v2DurationSeconds(placement.Fields["cache_ttl"]),
			"fallback":                placement.Fields["fallback_behavior"],
		})
	}
	return result
}

func v2DurationSeconds(value any) any {
	duration, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	unit, unitOK := duration["unit"].(string)
	amount, amountOK := duration["value"].(float64)
	if !unitOK || !amountOK {
		return nil
	}
	switch unit {
	case "seconds":
		return amount
	case "minutes":
		return amount * 60
	case "hours":
		return amount * 3600
	case "days":
		return amount * 86400
	default:
		return nil
	}
}

func sortedRecords(desired map[string]any, collection string) []entities.Record {
	records := entities.Records(desired, collection)
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records
}

func normalizedV2FrequencyValue(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	result := make(map[string]any, len(object))
	for key, field := range object {
		if key == "positions" {
			result[key] = normalizedPositions(field)
			continue
		}
		result[key] = normalizedV2FrequencyValue(field)
	}
	return result
}

func normalizedPositions(value any) any {
	if value == nil {
		return nil
	}
	items := []string{}
	switch values := value.(type) {
	case []any:
		for _, item := range values {
			if item, ok := item.(string); ok {
				items = append(items, item)
			}
		}
	case []string:
		items = append(items, values...)
	default:
		return value
	}
	sort.Strings(items)
	result := make([]any, 0, len(items))
	for _, item := range items {
		if len(result) == 0 || result[len(result)-1] != item {
			result = append(result, item)
		}
	}
	return result
}

func v2BindingSortKey(binding entities.Record) string {
	environmentID, _ := binding.Fields["environment_id"].(string)
	platform, _ := binding.Fields["platform"].(string)
	network, _ := binding.Fields["network"].(string)
	return environmentID + "\x00" + platform + "\x00" + network + "\x00" + binding.ID
}

func marshalV2JSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

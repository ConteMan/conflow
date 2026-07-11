// Package entities defines the runtime representation of Pack entity records.
package entities

// Record is the only entity representation persisted in Draft collections.
// Entity fields are deliberately nested so the stable entity ID cannot collide
// with a Pack-defined field name.
type Record struct {
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields"`
}

// Records reads one runtime entity collection. Invalid values are ignored so
// complete validation can report the remaining configuration deterministically.
func Records(configuration map[string]any, collection string) []Record {
	values, ok := configuration[collection].([]any)
	if !ok {
		return nil
	}
	result := make([]Record, 0, len(values))
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, _ := object["id"].(string)
		fields, _ := object["fields"].(map[string]any)
		if id != "" && fields != nil {
			result = append(result, Record{ID: id, Fields: cloneMap(fields)})
		}
	}
	return result
}

// Values serializes records in the runtime collection shape.
func Values(records []Record) []any {
	result := make([]any, len(records))
	for index, record := range records {
		result[index] = map[string]any{"id": record.ID, "fields": cloneMap(record.Fields)}
	}
	return result
}

// AdaptFlatFixture converts legacy golden fixture entities to the runtime
// shape. Contract fixtures predate entity CRUD and store {id, field...}; Draft
// collections store {id, fields:{field...}}. This is intentionally test-only
// input adaptation, not a second runtime shape accepted by Records.
func AdaptFlatFixture(configuration map[string]any) map[string]any {
	result := cloneMap(configuration)
	for collection, raw := range result {
		values, ok := raw.([]any)
		if !ok {
			continue
		}
		adapted := make([]any, 0, len(values))
		for _, value := range values {
			object, ok := value.(map[string]any)
			if !ok {
				adapted = append(adapted, value)
				continue
			}
			id, _ := object["id"].(string)
			if _, canonical := object["fields"].(map[string]any); canonical || id == "" {
				adapted = append(adapted, object)
				continue
			}
			fields := make(map[string]any, len(object)-1)
			for name, field := range object {
				if name != "id" {
					fields[name] = field
				}
			}
			adapted = append(adapted, map[string]any{"id": id, "fields": fields})
		}
		result[collection] = adapted
	}
	return result
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = cloneValue(item)
	}
	return result
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = cloneValue(item)
		}
		return result
	default:
		return value
	}
}

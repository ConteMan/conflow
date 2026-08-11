package validation

import (
	"bytes"
	"encoding/json"
	"regexp"

	"github.com/ConteMan/conflow/internal/packs"
)

func validatePackRules(input Input) []Diagnostic {
	schemas := make(map[string]packs.EntitySchema, len(input.Definition.Schema.Entities))
	for _, schema := range input.Definition.Schema.Entities {
		schemas[schema.Name] = schema
	}
	diagnostics := make([]Diagnostic, 0)
	for _, metadata := range input.Definition.Metadata.EntityTypes {
		schema, hasSchema := schemas[metadata.Name]
		pattern, patternErr := regexp.Compile(metadata.IDRule.Pattern)
		for _, record := range records(input.Effective, metadata.Collection) {
			ref := entityRef(input.PackRef, metadata.Name, record.ID)
			path := "/" + metadata.Collection + "/" + record.ID
			if patternErr != nil || len(record.ID) < metadata.IDRule.MinLength || len(record.ID) > metadata.IDRule.MaxLength || !pattern.MatchString(record.ID) {
				diagnostics = append(diagnostics, diagnostic("entity_id_invalid", path, SeverityError, ref, "实体 ID 不符合配置包规则", "在源配置中将实体 ID 改为符合配置包 ID 规则的稳定标识。"))
			}
			if !hasSchema {
				continue
			}
			for _, field := range schema.Fields {
				if len(field.Validation.Enum) == 0 {
					continue
				}
				value, exists := record.Fields[field.Name]
				if !exists || value == nil || enumAllows(value, field.Validation.Enum) {
					continue
				}
				diagnostics = append(diagnostics, diagnostic("field_value_not_allowed", path+"/"+field.Name, SeverityError, ref, "字段值不在配置包允许范围内", "将该字段改为配置包 schema 枚举中的允许值。"))
			}
		}
	}
	return diagnostics
}

func enumAllows(value any, allowed []json.RawMessage) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	for _, candidate := range allowed {
		if bytes.Equal(encoded, candidate) {
			return true
		}
	}
	return false
}

func mergePackAndDomainDiagnostics(packDiagnostics, domainDiagnostics []Diagnostic) []Diagnostic {
	domainPaths := make(map[string]bool, len(domainDiagnostics))
	for _, item := range domainDiagnostics {
		domainPaths[item.EntityRef+"\x00"+item.Path] = true
	}
	result := make([]Diagnostic, 0, len(packDiagnostics)+len(domainDiagnostics))
	for _, item := range packDiagnostics {
		if item.Code == "field_value_not_allowed" && domainPaths[item.EntityRef+"\x00"+item.Path] {
			continue
		}
		result = append(result, item)
	}
	return append(result, domainDiagnostics...)
}

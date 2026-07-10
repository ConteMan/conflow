package server

import (
	"encoding/json"

	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
)

type healthResponse struct {
	Status    string `json:"status"`
	ProjectID string `json:"project_id"`
}

type responseEnvelope struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}

type responseMeta struct {
	RequestID string `json:"request_id"`
	Revision  uint64 `json:"revision"`
}

type errorEnvelope struct {
	Error errorDTO `json:"error"`
}

type errorDTO struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	RequestID       string `json:"request_id"`
	CurrentRevision uint64 `json:"current_revision,omitempty"`
}

type bootstrapData struct {
	Project      projectDTO       `json:"project"`
	Environments []environmentDTO `json:"environments"`
	Capabilities capabilitiesDTO  `json:"capabilities"`
}

type capabilitiesDTO struct {
	ProjectEdit       bool `json:"project_edit"`
	EnvironmentManage bool `json:"environment_manage"`
}

type projectDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PackRef    string `json:"pack_ref"`
	SourceType string `json:"source_type"`
}

type updateProjectInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type environmentDTO struct {
	ID       string      `json:"id"`
	Provider providerDTO `json:"provider"`
	Publish  publishDTO  `json:"publish"`
}

type createEnvironmentInput struct {
	ID       string       `json:"id"`
	Provider providerDTO  `json:"provider"`
	Publish  publishInput `json:"publish"`
}

type updateEnvironmentInput struct {
	Provider providerDTO  `json:"provider"`
	Publish  publishInput `json:"publish"`
}

type providerDTO struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
}

type publishDTO struct {
	RequiresConfirmation bool `json:"requires_confirmation"`
}

// publishInput uses a pointer so an omitted required boolean is not silently
// interpreted as false by encoding/json.
type publishInput struct {
	RequiresConfirmation *bool `json:"requires_confirmation"`
}

type deleteEnvironmentData struct {
	DeletedID string `json:"deleted_id"`
}

type packSummaryDTO struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type packMetadataDTO struct {
	Ref           string              `json:"ref"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Description   string              `json:"description"`
	Capabilities  []string            `json:"capabilities"`
	SchemaVersion uint64              `json:"schema_version"`
	EntityTypes   []entityMetadataDTO `json:"entity_types"`
}

type entityMetadataDTO struct {
	Name                      string    `json:"name"`
	Label                     string    `json:"label"`
	Description               string    `json:"description"`
	IDRule                    idRuleDTO `json:"id_rule"`
	DeletionPolicy            string    `json:"deletion_policy"`
	EnvironmentOverrideFields []string  `json:"environment_override_fields"`
}

type idRuleDTO struct {
	Pattern   string `json:"pattern"`
	MinLength int    `json:"min_length"`
	MaxLength int    `json:"max_length"`
}

type packSchemaDTO struct {
	Version    uint64               `json:"version"`
	Entities   []entitySchemaDTO    `json:"entities"`
	Migrations []schemaMigrationDTO `json:"migrations"`
}

type entitySchemaDTO struct {
	Name   string           `json:"name"`
	Fields []fieldSchemaDTO `json:"fields"`
}

type fieldSchemaDTO struct {
	Name        string             `json:"name"`
	Type        string             `json:"type"`
	Required    bool               `json:"required"`
	Default     json.RawMessage    `json:"default"`
	Sensitivity string             `json:"sensitivity"`
	UI          fieldUIDTO         `json:"ui"`
	Validation  fieldValidationDTO `json:"validation"`
}

type fieldUIDTO struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Control     string `json:"control"`
	Group       string `json:"group"`
	Order       int    `json:"order"`
	Placeholder string `json:"placeholder,omitempty"`
}

type fieldValidationDTO struct {
	Enum      []json.RawMessage `json:"enum"`
	MinLength *int              `json:"min_length,omitempty"`
	MaxLength *int              `json:"max_length,omitempty"`
	Minimum   *float64          `json:"minimum,omitempty"`
	Maximum   *float64          `json:"maximum,omitempty"`
}

type schemaMigrationDTO struct {
	FromVersion uint64 `json:"from_version"`
	ToVersion   uint64 `json:"to_version"`
	Description string `json:"description"`
}

func projectDTOFrom(manifest project.Manifest) projectDTO {
	return projectDTO{
		ID:         manifest.Project.ID,
		Name:       manifest.Project.Name,
		PackRef:    manifest.Pack.ID,
		SourceType: manifest.Source.Type,
	}
}

func environmentsDTOFrom(environments []project.Environment) []environmentDTO {
	result := make([]environmentDTO, len(environments))
	for index, environment := range environments {
		result[index] = environmentDTOFrom(environment)
	}
	return result
}

func environmentDTOFrom(environment project.Environment) environmentDTO {
	return environmentDTO{
		ID: environment.ID,
		Provider: providerDTO{
			Type:      environment.Provider.Type,
			ProjectID: environment.Provider.ProjectID,
		},
		Publish: publishDTO{RequiresConfirmation: environment.Publish.RequiresConfirmation},
	}
}

func packSummariesDTOFrom(definitions []packs.Definition) []packSummaryDTO {
	result := make([]packSummaryDTO, len(definitions))
	for index, definition := range definitions {
		result[index] = packSummaryDTO{
			Ref:         packs.Reference{Name: definition.Metadata.Name, Version: definition.Metadata.Version}.String(),
			Name:        definition.Metadata.Name,
			Version:     definition.Metadata.Version,
			Description: definition.Metadata.Description,
		}
	}
	return result
}

func packMetadataDTOFrom(definition packs.Definition) packMetadataDTO {
	result := packMetadataDTO{
		Ref:           packs.Reference{Name: definition.Metadata.Name, Version: definition.Metadata.Version}.String(),
		Name:          definition.Metadata.Name,
		Version:       definition.Metadata.Version,
		Description:   definition.Metadata.Description,
		Capabilities:  append([]string{}, definition.Metadata.Capabilities...),
		SchemaVersion: definition.Schema.Version,
		EntityTypes:   make([]entityMetadataDTO, len(definition.Metadata.EntityTypes)),
	}
	for index, entity := range definition.Metadata.EntityTypes {
		result.EntityTypes[index] = entityMetadataDTO{
			Name:                      entity.Name,
			Label:                     entity.Label,
			Description:               entity.Description,
			IDRule:                    idRuleDTO{Pattern: entity.IDRule.Pattern, MinLength: entity.IDRule.MinLength, MaxLength: entity.IDRule.MaxLength},
			DeletionPolicy:            string(entity.DeletionPolicy),
			EnvironmentOverrideFields: append([]string{}, entity.EnvironmentOverrideFields...),
		}
	}
	return result
}

func packSchemaDTOFrom(schema packs.Schema) packSchemaDTO {
	result := packSchemaDTO{
		Version:    schema.Version,
		Entities:   make([]entitySchemaDTO, len(schema.Entities)),
		Migrations: make([]schemaMigrationDTO, len(schema.Migrations)),
	}
	for entityIndex, entity := range schema.Entities {
		fields := make([]fieldSchemaDTO, len(entity.Fields))
		for fieldIndex, field := range entity.Fields {
			fields[fieldIndex] = fieldSchemaDTO{
				Name:        field.Name,
				Type:        string(field.Type),
				Required:    field.Required,
				Default:     append(json.RawMessage(nil), field.Default...),
				Sensitivity: string(field.Sensitivity),
				UI:          fieldUIDTO{Label: field.UI.Label, Description: field.UI.Description, Control: field.UI.Control, Group: field.UI.Group, Order: field.UI.Order, Placeholder: field.UI.Placeholder},
				Validation: fieldValidationDTO{
					Enum:      cloneRawMessagesDTO(field.Validation.Enum),
					MinLength: field.Validation.MinLength,
					MaxLength: field.Validation.MaxLength,
					Minimum:   field.Validation.Minimum,
					Maximum:   field.Validation.Maximum,
				},
			}
		}
		result.Entities[entityIndex] = entitySchemaDTO{Name: entity.Name, Fields: fields}
	}
	for index, migration := range schema.Migrations {
		result.Migrations[index] = schemaMigrationDTO{FromVersion: migration.FromVersion, ToVersion: migration.ToVersion, Description: migration.Description}
	}
	return result
}

func cloneRawMessagesDTO(values []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		result[index] = append(json.RawMessage(nil), value...)
	}
	return result
}

func (environment createEnvironmentInput) toProject() (project.Environment, bool) {
	publish, ok := environment.Publish.toProject()
	if !ok {
		return project.Environment{}, false
	}
	return project.Environment{
		ID:       environment.ID,
		Provider: environment.Provider.toProject(),
		Publish:  publish,
	}, true
}

func (provider providerDTO) toProject() project.Provider {
	return project.Provider{Type: provider.Type, ProjectID: provider.ProjectID}
}

func (publish publishInput) toProject() (project.Publish, bool) {
	if publish.RequiresConfirmation == nil {
		return project.Publish{}, false
	}
	return project.Publish{RequiresConfirmation: *publish.RequiresConfirmation}, true
}

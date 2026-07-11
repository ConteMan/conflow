package server

import (
	"encoding/json"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/release"
	"github.com/ConteMan/conflow/internal/validation"
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

type manifestRevisionMismatchEnvelope struct {
	Error manifestRevisionMismatchDTO `json:"error"`
}

type manifestRevisionMismatchDTO struct {
	Code            string           `json:"code"`
	Message         string           `json:"message"`
	RequestID       string           `json:"request_id"`
	CurrentRevision uint64           `json:"current_revision"`
	CurrentState    manifestStateDTO `json:"current_state"`
}

type manifestStateDTO struct {
	Project      projectDTO       `json:"project"`
	Environments []environmentDTO `json:"environments"`
}

type errorDTO struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	RequestID       string `json:"request_id"`
	CurrentRevision uint64 `json:"current_revision,omitempty"`
}

type remoteValidateInput struct {
	PlanID string `json:"plan_id"`
}

type createReleaseInput struct {
	PlanID                string                 `json:"plan_id"`
	ExpectedDraftRevision uint64                 `json:"expected_draft_revision"`
	ExpectedRemoteETag    string                 `json:"expected_remote_etag"`
	Confirmation          releaseConfirmationDTO `json:"confirmation"`
}
type releaseConfirmationDTO struct {
	Acknowledged            *bool    `json:"acknowledged"`
	EnvironmentID           string   `json:"environment_id"`
	AcknowledgedRiskItemIDs []string `json:"acknowledged_risk_item_ids"`
}

func (input createReleaseInput) valid() bool {
	return input.PlanID != "" && input.ExpectedDraftRevision > 0 && input.ExpectedRemoteETag != "" && input.Confirmation.Acknowledged != nil && input.Confirmation.AcknowledgedRiskItemIDs != nil
}

type remoteETagMismatchEnvelope struct {
	Error remoteETagMismatchDTO `json:"error"`
}
type remoteETagMismatchDTO struct {
	Code               string         `json:"code"`
	Message            string         `json:"message"`
	RequestID          string         `json:"request_id"`
	PlanID             string         `json:"plan_id"`
	ExpectedRemoteETag string         `json:"expected_remote_etag"`
	CurrentRemote      remoteAuditDTO `json:"current_remote"`
	Rebuild            rebuildDTO     `json:"rebuild"`
}
type remoteAuditDTO struct {
	RemoteETag string `json:"remote_etag"`
	Version    string `json:"version"`
	ObservedAt string `json:"observed_at"`
	Summary    any    `json:"summary"`
}
type rebuildDTO struct {
	Required     bool   `json:"required"`
	PlanEndpoint string `json:"plan_endpoint"`
	ReasonCode   string `json:"reason_code"`
}

type releaseDTO = release.Release

type replaceDraftInput struct {
	ExpectedSourceRevision *string         `json:"expected_source_revision"`
	WriteScope             string          `json:"write_scope"`
	Configuration          json.RawMessage `json:"configuration"`
}

func (input replaceDraftInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != "" && input.WriteScope != "" && input.Configuration != nil
}

type draftScopeMutationInput struct {
	ExpectedSourceRevision *string `json:"expected_source_revision"`
	WriteScope             string  `json:"write_scope"`
}

type saveDraftInput struct {
	ExpectedSourceRevision *string `json:"expected_source_revision"`
}

func (input saveDraftInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != ""
}

type createEntityInput struct {
	ExpectedSourceRevision *string           `json:"expected_source_revision"`
	WriteScope             string            `json:"write_scope"`
	EntityType             string            `json:"entity_type"`
	Entity                 *app.EntityRecord `json:"entity"`
}

func (input createEntityInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != "" && input.WriteScope != "" && input.EntityType != "" && input.Entity != nil && input.Entity.ID != ""
}

type entityMutationInput struct {
	ExpectedSourceRevision *string           `json:"expected_source_revision"`
	WriteScope             string            `json:"write_scope"`
	Entity                 *app.EntityRecord `json:"entity"`
}

func (input entityMutationInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != "" && input.WriteScope != "" && input.Entity != nil && input.Entity.ID != ""
}

type entityDeleteInput struct {
	ExpectedSourceRevision *string `json:"expected_source_revision"`
	WriteScope             string  `json:"write_scope"`
}

func (input entityDeleteInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != "" && input.WriteScope != ""
}

type entityReferencedEnvelope struct {
	Error entityReferencedDTO `json:"error"`
}

type entityReferencedDTO struct {
	Code            string                `json:"code"`
	Message         string                `json:"message"`
	RequestID       string                `json:"request_id"`
	CurrentRevision uint64                `json:"current_revision"`
	References      []app.EntityReference `json:"references"`
}

func (input draftScopeMutationInput) valid() bool {
	return input.ExpectedSourceRevision != nil && *input.ExpectedSourceRevision != "" && input.WriteScope != ""
}

type draftViewDTO = draft.View

func draftViewDTOFrom(view draft.View) draftViewDTO { return view }

type validationResultDTO = validation.Result

func validationResultDTOFrom(result validation.Result) validationResultDTO { return result }

type draftConflictEnvelope struct {
	Error draftConflictDTO `json:"error"`
}
type draftConflictDTO struct {
	Code                  string       `json:"code"`
	Message               string       `json:"message"`
	RequestID             string       `json:"request_id"`
	CurrentRevision       uint64       `json:"current_revision"`
	CurrentSourceRevision string       `json:"current_source_revision"`
	ConflictScope         string       `json:"conflict_scope"`
	CurrentState          draftViewDTO `json:"current_state"`
}

type draftValidationEnvelope struct {
	Error draftValidationDTO `json:"error"`
}
type draftValidationDTO struct {
	Code      string                  `json:"code"`
	Message   string                  `json:"message"`
	RequestID string                  `json:"request_id"`
	Details   []draft.StructuralError `json:"details"`
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

type sourceDTO struct {
	Type         string                `json:"type"`
	Capabilities sourceCapabilitiesDTO `json:"capabilities"`
}

type sourceCapabilitiesDTO struct {
	Load bool `json:"load"`
	Save bool `json:"save"`
}

type sourceStatusDTO struct {
	Type             string   `json:"type"`
	Digest           string   `json:"digest"`
	ExternalModified bool     `json:"external_modified"`
	Paths            []string `json:"paths"`
}

type projectDTO struct {
	ID                        string                            `json:"id"`
	Name                      string                            `json:"name"`
	PackRef                   string                            `json:"pack_ref"`
	SourceType                string                            `json:"source_type"`
	ReleaseConfirmationPolicy project.ReleaseConfirmationPolicy `json:"release_confirmation_policy,omitempty"`
}

type updateProjectInput struct {
	ID                        string                            `json:"id"`
	Name                      string                            `json:"name"`
	ReleaseConfirmationPolicy project.ReleaseConfirmationPolicy `json:"release_confirmation_policy"`
}

type environmentDTO struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Provider providerDTO `json:"provider"`
	Publish  publishDTO  `json:"publish"`
}

type createEnvironmentInput struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	Provider providerDTO  `json:"provider"`
	Publish  publishInput `json:"publish"`
}

type updateEnvironmentInput struct {
	Name     string       `json:"name"`
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
	Nullable    bool               `json:"nullable"`
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
		ID:                        manifest.Project.ID,
		Name:                      manifest.Project.Name,
		PackRef:                   manifest.Pack.ID,
		SourceType:                manifest.Source.Type,
		ReleaseConfirmationPolicy: manifest.Project.ReleaseConfirmationPolicy,
	}
}

func sourceDTOFrom(info app.SourceInfo) sourceDTO {
	return sourceDTO{Type: info.Type, Capabilities: sourceCapabilitiesDTO{Load: info.Capabilities.Read, Save: info.Capabilities.Save}}
}

func sourceStatusDTOFrom(info app.SourceInfo) sourceStatusDTO {
	return sourceStatusDTO{Type: info.Status.Type, Digest: info.Status.Digest, ExternalModified: info.Status.ExternalModified, Paths: append([]string{}, info.Status.Paths...)}
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
		ID:   environment.ID,
		Name: environment.Name,
		Kind: environment.Kind,
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
				Nullable:    field.Nullable,
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
		Name:     environment.Name,
		Kind:     environment.Kind,
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

package server

import "github.com/ConteMan/conflow/internal/project"

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

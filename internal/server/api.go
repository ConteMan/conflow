package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/validation"
)

type api struct {
	service *app.Service
}

func newAPI(service *app.Service) *api {
	return &api{service: service}
}

func (a *api) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /api/v1/bootstrap", a.bootstrap)
	mux.HandleFunc("GET /api/v1/project", a.getProject)
	mux.HandleFunc("PUT /api/v1/project", a.updateProject)
	mux.HandleFunc("GET /api/v1/environments", a.listEnvironments)
	mux.HandleFunc("POST /api/v1/environments", a.createEnvironment)
	mux.HandleFunc("GET /api/v1/environments/{environment_id}", a.getEnvironment)
	mux.HandleFunc("PUT /api/v1/environments/{environment_id}", a.updateEnvironment)
	mux.HandleFunc("DELETE /api/v1/environments/{environment_id}", a.deleteEnvironment)
	mux.HandleFunc("GET /api/v1/packs", a.listPacks)
	mux.HandleFunc("GET /api/v1/packs/{pack_name}/versions/{pack_version}", a.getPack)
	mux.HandleFunc("GET /api/v1/packs/{pack_name}/versions/{pack_version}/schema", a.getPackSchema)
	mux.HandleFunc("GET /api/v1/drafts/{environment_id}", a.getDraft)
	mux.HandleFunc("PUT /api/v1/drafts/{environment_id}", a.replaceDraft)
	mux.HandleFunc("GET /api/v1/drafts/{environment_id}/diagnostics", a.getDiagnostics)
	mux.HandleFunc("GET /api/v1/drafts/{environment_id}/entities", a.listEntities)
	mux.HandleFunc("POST /api/v1/drafts/{environment_id}/entities", a.createEntity)
	mux.HandleFunc("GET /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}", a.getEntity)
	mux.HandleFunc("PUT /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}", a.replaceEntity)
	mux.HandleFunc("DELETE /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}", a.deleteEntity)
	mux.HandleFunc("GET /api/v1/drafts/{environment_id}/entities/{entity_type}/{entity_id}/referenced-by", a.getEntityReferences)
	mux.HandleFunc("POST /api/v1/drafts/", a.draftAction)
	mux.HandleFunc("/api/v1/project", methodNotAllowed)
	mux.HandleFunc("/api/v1/environments", methodNotAllowed)
	mux.HandleFunc("/api/v1/environments/{environment_id}", methodNotAllowed)
	mux.HandleFunc("/api/v1/packs", methodNotAllowed)
	mux.HandleFunc("/api/v1/packs/{pack_name}/versions/{pack_version}", methodNotAllowed)
	mux.HandleFunc("/api/v1/packs/{pack_name}/versions/{pack_version}/schema", methodNotAllowed)
	mux.HandleFunc("/api/", routeNotFound)
	return apiMiddleware(mux)
}

func methodNotAllowed(writer http.ResponseWriter, request *http.Request) {
	writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", 0)
}

func routeNotFound(writer http.ResponseWriter, request *http.Request) {
	writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "API 路径不存在", 0)
}

func (a *api) health(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := a.service.Snapshot(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "project_unavailable", "项目配置当前不可用", 0)
		return
	}
	writeJSON(writer, http.StatusOK, healthResponse{Status: "ok", ProjectID: snapshot.Manifest.Project.ID})
}

func (a *api) bootstrap(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := a.service.Snapshot(request.Context())
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	data := bootstrapData{
		Project:      projectDTOFrom(snapshot.Manifest),
		Environments: environmentsDTOFrom(snapshot.Manifest.Environments),
		Capabilities: capabilitiesDTO{ProjectEdit: true, EnvironmentManage: true},
	}
	writeSuccess(writer, request, http.StatusOK, data, snapshot.Revision)
}

func (a *api) getProject(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := a.service.Snapshot(request.Context())
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, projectDTOFrom(snapshot.Manifest), snapshot.Revision)
}

func (a *api) updateProject(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input updateProjectInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeRequestError(writer, request, err)
		return
	}
	snapshot, err := a.service.UpdateProject(request.Context(), revision, project.Project{ID: input.ID, Name: input.Name})
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, projectDTOFrom(snapshot.Manifest), snapshot.Revision)
}

func (a *api) listEnvironments(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := a.service.Snapshot(request.Context())
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, environmentsDTOFrom(snapshot.Manifest.Environments), snapshot.Revision)
}

func (a *api) createEnvironment(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input createEnvironmentInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeRequestError(writer, request, err)
		return
	}
	environment, ok := input.toProject()
	if !ok {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "项目配置不合法", 0)
		return
	}
	snapshot, environment, err := a.service.CreateEnvironment(request.Context(), revision, environment)
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusCreated, environmentDTOFrom(environment), snapshot.Revision)
}

func (a *api) getEnvironment(writer http.ResponseWriter, request *http.Request) {
	snapshot, environment, err := a.service.GetEnvironment(request.Context(), request.PathValue("environment_id"))
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, environmentDTOFrom(environment), snapshot.Revision)
}

func (a *api) updateEnvironment(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input updateEnvironmentInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeRequestError(writer, request, err)
		return
	}
	publish, ok := input.Publish.toProject()
	if !ok {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "项目配置不合法", 0)
		return
	}
	environmentID := request.PathValue("environment_id")
	replacement := project.Environment{
		ID:       environmentID,
		Name:     input.Name,
		Provider: input.Provider.toProject(),
		Publish:  publish,
	}
	snapshot, environment, err := a.service.UpdateEnvironment(request.Context(), revision, environmentID, replacement)
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, environmentDTOFrom(environment), snapshot.Revision)
}

func (a *api) deleteEnvironment(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	environmentID := request.PathValue("environment_id")
	snapshot, err := a.service.DeleteEnvironment(request.Context(), revision, environmentID)
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, deleteEnvironmentData{DeletedID: environmentID}, snapshot.Revision)
}

func (a *api) listPacks(writer http.ResponseWriter, request *http.Request) {
	snapshot := a.service.ListPacks(request.Context())
	writeSuccess(writer, request, http.StatusOK, packSummariesDTOFrom(snapshot.Definitions), snapshot.Revision)
}

func (a *api) getPack(writer http.ResponseWriter, request *http.Request) {
	definition, revision, err := a.service.GetPack(request.Context(), request.PathValue("pack_name"), request.PathValue("pack_version"))
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, packMetadataDTOFrom(definition), revision)
}

func (a *api) getPackSchema(writer http.ResponseWriter, request *http.Request) {
	requestedVersion, err := schemaVersionFrom(request)
	if err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "schema_version 必须是正整数", 0)
		return
	}
	schema, revision, err := a.service.GetPackSchema(request.Context(), request.PathValue("pack_name"), request.PathValue("pack_version"), requestedVersion)
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, packSchemaDTOFrom(schema), revision)
}

func (a *api) getDraft(writer http.ResponseWriter, request *http.Request) {
	view, revision, err := a.service.GetDraft(request.Context(), request.PathValue("environment_id"))
	if err != nil {
		a.writeError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, draftViewDTOFrom(view), revision)
}

func (a *api) validateDraft(writer http.ResponseWriter, request *http.Request) {
	result, revision, err := a.service.ValidateDraft(request.Context(), request.PathValue("environment_id"))
	if err != nil {
		a.writeValidationError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, validationResultDTOFrom(result), revision)
}

func (a *api) getDiagnostics(writer http.ResponseWriter, request *http.Request) {
	result, revision, err := a.service.Diagnostics(request.Context(), request.PathValue("environment_id"))
	if err != nil {
		a.writeValidationError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, validationResultDTOFrom(result), revision)
}

func (a *api) listEntities(writer http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()["entity_type"]
	if len(values) > 1 {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "entity_type 只能提供一次", 0)
		return
	}
	entityType := ""
	if len(values) == 1 {
		entityType = values[0]
	}
	entities, revision, err := a.service.ListEntities(request.Context(), request.PathValue("environment_id"), entityType)
	if err != nil {
		a.writeEntityError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, entities, revision)
}

func (a *api) getEntity(writer http.ResponseWriter, request *http.Request) {
	entity, revision, err := a.service.GetEntity(request.Context(), request.PathValue("environment_id"), request.PathValue("entity_type"), request.PathValue("entity_id"))
	if err != nil {
		a.writeEntityError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, entity, revision)
}

func (a *api) getEntityReferences(writer http.ResponseWriter, request *http.Request) {
	references, revision, err := a.service.GetEntityReferences(request.Context(), request.PathValue("environment_id"), request.PathValue("entity_type"), request.PathValue("entity_id"))
	if err != nil {
		a.writeEntityError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, references, revision)
}

func (a *api) createEntity(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input createEntityInput
	if err := decodeJSON(writer, request, &input); err != nil || !input.valid() {
		writeRequestError(writer, request, err)
		return
	}
	a.mutateEntity(writer, request, http.StatusCreated, app.EntityMutation{ExpectedRevision: revision, ExpectedSourceRevision: *input.ExpectedSourceRevision, Scope: input.WriteScope, EntityType: input.EntityType, EntityID: input.Entity.ID, Entity: input.Entity, Action: "create"})
}

func (a *api) replaceEntity(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input entityMutationInput
	if err := decodeJSON(writer, request, &input); err != nil || !input.valid() {
		writeRequestError(writer, request, err)
		return
	}
	a.mutateEntity(writer, request, http.StatusOK, app.EntityMutation{ExpectedRevision: revision, ExpectedSourceRevision: *input.ExpectedSourceRevision, Scope: input.WriteScope, EntityType: request.PathValue("entity_type"), EntityID: request.PathValue("entity_id"), Entity: input.Entity, Action: "replace"})
}

func (a *api) deleteEntity(writer http.ResponseWriter, request *http.Request) {
	if !hasJSONContentType(request.Header.Get("Content-Type")) {
		writeAPIError(writer, request, http.StatusUnsupportedMediaType, "unsupported_media_type", "请求必须使用 application/json", 0)
		return
	}
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input entityDeleteInput
	if err := decodeJSON(writer, request, &input); err != nil || !input.valid() {
		writeRequestError(writer, request, err)
		return
	}
	a.mutateEntity(writer, request, http.StatusOK, app.EntityMutation{ExpectedRevision: revision, ExpectedSourceRevision: *input.ExpectedSourceRevision, Scope: input.WriteScope, EntityType: request.PathValue("entity_type"), EntityID: request.PathValue("entity_id"), Action: "delete"})
}

func (a *api) mutateEntity(writer http.ResponseWriter, request *http.Request, status int, mutation app.EntityMutation) {
	entity, revision, err := a.service.MutateEntity(request.Context(), request.PathValue("environment_id"), mutation)
	if err != nil {
		a.writeEntityError(writer, request, err)
		return
	}
	writeSuccess(writer, request, status, entity, revision)
}

func (a *api) replaceDraft(writer http.ResponseWriter, request *http.Request) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input replaceDraftInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeRequestError(writer, request, err)
		return
	}
	if !input.valid() {
		writeRequestError(writer, request, errors.New("missing draft request field"))
		return
	}
	a.mutateDraft(writer, request, request.PathValue("environment_id"), revision, *input.ExpectedSourceRevision, input.WriteScope, "put", input.Configuration)
}

func (a *api) draftAction(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/drafts/")
	for suffix, action := range map[string]string{":reset": "reset", ":discard": "discard", ":validate": "validate"} {
		if strings.HasSuffix(path, suffix) {
			environmentID := strings.TrimSuffix(path, suffix)
			if environmentID == "" || strings.Contains(environmentID, "/") {
				routeNotFound(writer, request)
				return
			}
			if action == "validate" {
				request.SetPathValue("environment_id", environmentID)
				a.validateDraft(writer, request)
				return
			}
			a.mutateDraftAction(writer, request, environmentID, action)
			return
		}
	}
	routeNotFound(writer, request)
}

func (a *api) mutateDraftAction(writer http.ResponseWriter, request *http.Request, environmentID, action string) {
	revision, ok := requireRevision(writer, request)
	if !ok {
		return
	}
	var input draftScopeMutationInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeRequestError(writer, request, err)
		return
	}
	if !input.valid() {
		writeRequestError(writer, request, errors.New("missing draft request field"))
		return
	}
	a.mutateDraft(writer, request, environmentID, revision, *input.ExpectedSourceRevision, input.WriteScope, action, nil)
}

func (a *api) mutateDraft(writer http.ResponseWriter, request *http.Request, environmentID string, revision uint64, expectedSourceRevision, scope, action string, configuration []byte) {
	view, nextRevision, err := a.service.MutateDraft(request.Context(), environmentID, draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: expectedSourceRevision, Scope: scope, Action: action, Configuration: configuration})
	if err != nil {
		a.writeDraftError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, draftViewDTOFrom(view), nextRevision)
}

func schemaVersionFrom(request *http.Request) (*uint64, error) {
	values, exists := request.URL.Query()["schema_version"]
	if !exists {
		return nil, nil
	}
	if len(values) != 1 || values[0] == "" {
		return nil, errors.New("invalid schema version")
	}
	raw := values[0]
	version, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || version == 0 {
		return nil, errors.New("invalid schema version")
	}
	return &version, nil
}

func (a *api) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var mismatch *project.RevisionMismatchError
	switch {
	case errors.As(err, &mismatch):
		writeManifestRevisionMismatch(writer, request, mismatch.Snapshot)
	case errors.Is(err, project.ErrNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "environment_not_found", "环境不存在", 0)
	case errors.Is(err, project.ErrAlreadyExists):
		writeAPIError(writer, request, http.StatusConflict, "environment_already_exists", "环境 ID 已存在", 0)
	case errors.Is(err, app.ErrLastEnvironment):
		writeAPIError(writer, request, http.StatusConflict, "last_environment", "项目必须至少保留一个环境", 0)
	case errors.Is(err, packs.ErrUnknownPack):
		writeAPIError(writer, request, http.StatusNotFound, "pack_not_found", "配置包不存在", 0)
	case errors.Is(err, packs.ErrUnknownVersion):
		writeAPIError(writer, request, http.StatusNotFound, "pack_version_not_found", "配置包版本不存在", 0)
	case errors.Is(err, packs.ErrSchemaIncompatible):
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "schema_incompatible", "配置包 schema 版本不兼容", 0)
	case errors.Is(err, project.ErrInvalidManifest):
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "项目配置不合法", 0)
	case errors.Is(err, draft.ErrEnvironmentNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "environment_not_found", "环境不存在", 0)
	default:
		writeAPIError(writer, request, http.StatusInternalServerError, "internal_error", "操作失败", 0)
	}
}

func (a *api) writeDraftError(writer http.ResponseWriter, request *http.Request, err error) {
	var conflict *draft.ConflictError
	var validation *draft.ValidationError
	switch {
	case errors.As(err, &conflict):
		writeDraftConflict(writer, request, conflict)
	case errors.As(err, &validation):
		writeDraftValidationError(writer, request, validation.Details)
	case errors.Is(err, draft.ErrInvalidScope):
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "write_scope 不合法", 0)
	default:
		a.writeError(writer, request, err)
	}
}

func (a *api) writeEntityError(writer http.ResponseWriter, request *http.Request, err error) {
	var referenced *app.EntityReferencedError
	switch {
	case errors.As(err, &referenced):
		writeEntityReferenced(writer, request, referenced.Revision, referenced.References)
	case errors.Is(err, app.ErrEntityNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "entity_not_found", "实体不存在", 0)
	case errors.Is(err, app.ErrEntityTypeInvalid), errors.Is(err, app.ErrEntityIDInvalid), errors.Is(err, app.ErrEntityIDImmutable), errors.Is(err, app.ErrEntityExists):
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_request", "实体请求不合法", 0)
	default:
		a.writeDraftError(writer, request, err)
	}
}

func (a *api) writeValidationError(writer http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, validation.ErrNotFound) {
		writeAPIError(writer, request, http.StatusNotFound, "validation_not_found", "尚未运行完整校验", 0)
		return
	}
	a.writeError(writer, request, err)
}

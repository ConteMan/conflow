package server

import (
	"errors"
	"net/http"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/project"
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
	mux.HandleFunc("/api/v1/project", methodNotAllowed)
	mux.HandleFunc("/api/v1/environments", methodNotAllowed)
	mux.HandleFunc("/api/v1/environments/{environment_id}", methodNotAllowed)
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

func (a *api) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var mismatch *project.RevisionMismatchError
	switch {
	case errors.As(err, &mismatch):
		writeAPIError(writer, request, http.StatusPreconditionFailed, "revision_mismatch", "项目已被其他操作修改，请重新加载", mismatch.Current)
	case errors.Is(err, project.ErrNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "environment_not_found", "环境不存在", 0)
	case errors.Is(err, project.ErrAlreadyExists):
		writeAPIError(writer, request, http.StatusConflict, "environment_already_exists", "环境 ID 已存在", 0)
	case errors.Is(err, app.ErrLastEnvironment):
		writeAPIError(writer, request, http.StatusConflict, "last_environment", "项目必须至少保留一个环境", 0)
	case errors.Is(err, project.ErrInvalidManifest):
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "项目配置不合法", 0)
	default:
		writeAPIError(writer, request, http.StatusInternalServerError, "internal_error", "操作失败", 0)
	}
}

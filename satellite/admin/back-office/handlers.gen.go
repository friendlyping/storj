// AUTOGENERATED BY private/apigen
// DO NOT EDIT.

package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spacemonkeygo/monkit/v3"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/uuid"
	"storj.io/storj/private/api"
)

var ErrPlacementsAPI = errs.Class("admin placements api")
var ErrUsersAPI = errs.Class("admin users api")
var ErrProjectsAPI = errs.Class("admin projects api")

type PlacementManagementService interface {
	GetPlacements(ctx context.Context) ([]PlacementInfo, api.HTTPError)
}

type UserManagementService interface {
	GetUserByEmail(ctx context.Context, email string) (*UserAccount, api.HTTPError)
}

type ProjectManagementService interface {
	GetProject(ctx context.Context, publicID uuid.UUID) (*Project, api.HTTPError)
	UpdateProjectLimits(ctx context.Context, publicID uuid.UUID, request ProjectLimitsUpdate) api.HTTPError
}

// PlacementManagementHandler is an api handler that implements all PlacementManagement API endpoints functionality.
type PlacementManagementHandler struct {
	log     *zap.Logger
	mon     *monkit.Scope
	service PlacementManagementService
}

// UserManagementHandler is an api handler that implements all UserManagement API endpoints functionality.
type UserManagementHandler struct {
	log     *zap.Logger
	mon     *monkit.Scope
	service UserManagementService
	auth    *Authorizer
}

// ProjectManagementHandler is an api handler that implements all ProjectManagement API endpoints functionality.
type ProjectManagementHandler struct {
	log     *zap.Logger
	mon     *monkit.Scope
	service ProjectManagementService
	auth    *Authorizer
}

func NewPlacementManagement(log *zap.Logger, mon *monkit.Scope, service PlacementManagementService, router *mux.Router) *PlacementManagementHandler {
	handler := &PlacementManagementHandler{
		log:     log,
		mon:     mon,
		service: service,
	}

	placementsRouter := router.PathPrefix("/back-office/api/v1/placements").Subrouter()
	placementsRouter.HandleFunc("/", handler.handleGetPlacements).Methods("GET")

	return handler
}

func NewUserManagement(log *zap.Logger, mon *monkit.Scope, service UserManagementService, router *mux.Router, auth *Authorizer) *UserManagementHandler {
	handler := &UserManagementHandler{
		log:     log,
		mon:     mon,
		service: service,
		auth:    auth,
	}

	usersRouter := router.PathPrefix("/back-office/api/v1/users").Subrouter()
	usersRouter.HandleFunc("/{email}", handler.handleGetUserByEmail).Methods("GET")

	return handler
}

func NewProjectManagement(log *zap.Logger, mon *monkit.Scope, service ProjectManagementService, router *mux.Router, auth *Authorizer) *ProjectManagementHandler {
	handler := &ProjectManagementHandler{
		log:     log,
		mon:     mon,
		service: service,
		auth:    auth,
	}

	projectsRouter := router.PathPrefix("/back-office/api/v1/projects").Subrouter()
	projectsRouter.HandleFunc("/{publicID}", handler.handleGetProject).Methods("GET")
	projectsRouter.HandleFunc("/limits/{publicID}", handler.handleUpdateProjectLimits).Methods("PUT")

	return handler
}

func (h *PlacementManagementHandler) handleGetPlacements(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer h.mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	retVal, httpErr := h.service.GetPlacements(ctx)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GetPlacements response", zap.Error(ErrPlacementsAPI.Wrap(err)))
	}
}

func (h *UserManagementHandler) handleGetUserByEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer h.mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	email, ok := mux.Vars(r)["email"]
	if !ok {
		api.ServeError(h.log, w, http.StatusBadRequest, errs.New("missing email route param"))
		return
	}

	if h.auth.IsRejected(w, r, 1) {
		return
	}

	retVal, httpErr := h.service.GetUserByEmail(ctx, email)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GetUserByEmail response", zap.Error(ErrUsersAPI.Wrap(err)))
	}
}

func (h *ProjectManagementHandler) handleGetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer h.mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	publicIDParam, ok := mux.Vars(r)["publicID"]
	if !ok {
		api.ServeError(h.log, w, http.StatusBadRequest, errs.New("missing publicID route param"))
		return
	}

	publicID, err := uuid.FromString(publicIDParam)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	if h.auth.IsRejected(w, r, 8192) {
		return
	}

	retVal, httpErr := h.service.GetProject(ctx, publicID)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GetProject response", zap.Error(ErrProjectsAPI.Wrap(err)))
	}
}

func (h *ProjectManagementHandler) handleUpdateProjectLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer h.mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	publicIDParam, ok := mux.Vars(r)["publicID"]
	if !ok {
		api.ServeError(h.log, w, http.StatusBadRequest, errs.New("missing publicID route param"))
		return
	}

	publicID, err := uuid.FromString(publicIDParam)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	payload := ProjectLimitsUpdate{}
	if err = json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	if h.auth.IsRejected(w, r, 16384) {
		return
	}

	httpErr := h.service.UpdateProjectLimits(ctx, publicID, payload)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
	}
}

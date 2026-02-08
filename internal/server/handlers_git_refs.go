package server

import (
	"fmt"
	"net/http"
	"strings"

	"grns/internal/api"
	"grns/internal/models"
)

func (s *Server) handleCreateTaskGitRef(w http.ResponseWriter, r *http.Request) {
	if s.gitRefService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("git refs are not configured")))
		return
	}

	taskID, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	var req api.TaskGitRefCreateRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	ref, err := s.gitRefService.Create(r.Context(), taskID, req)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusCreated, ref)
}

func (s *Server) handleListTaskGitRefs(w http.ResponseWriter, r *http.Request) {
	if s.gitRefService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("git refs are not configured")))
		return
	}

	taskID, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	refs, err := s.gitRefService.List(r.Context(), taskID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if refs == nil {
		refs = []models.TaskGitRef{}
	}

	s.writeJSON(w, http.StatusOK, refs)
}

func (s *Server) handleGetTaskGitRef(w http.ResponseWriter, r *http.Request) {
	if s.gitRefService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("git refs are not configured")))
		return
	}

	refID, err := requireTaskGitRefID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	ref, err := s.gitRefService.Get(r.Context(), refID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, ref)
}

func (s *Server) handleDeleteTaskGitRef(w http.ResponseWriter, r *http.Request) {
	if s.gitRefService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("git refs are not configured")))
		return
	}

	refID, err := requireTaskGitRefID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.gitRefService.Delete(r.Context(), refID); err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"id": refID})
}

func requireTaskGitRefID(r *http.Request) (string, error) {
	id := strings.TrimSpace(r.PathValue("ref_id"))
	if !validateGitRefID(id) {
		return "", badRequestCode(fmt.Errorf("invalid ref_id"), ErrCodeInvalidID)
	}
	return id, nil
}

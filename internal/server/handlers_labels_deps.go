package server

import (
	"fmt"
	"net/http"
	"strings"

	"grns/internal/api"
	"grns/internal/models"
)

func (s *Server) handleDepTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	nodes, err := s.store.DependencyTree(r.Context(), id)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusInternalServerError, err)
		return
	}
	if nodes == nil {
		nodes = []models.DepTreeNode{}
	}

	s.writeJSON(w, http.StatusOK, api.DepTreeResponse{
		RootID: id,
		Nodes:  nodes,
	})
}

func (s *Server) handleDeps(w http.ResponseWriter, r *http.Request) {
	var req api.DepCreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	childID := strings.TrimSpace(req.ChildID)
	parentID := strings.TrimSpace(req.ParentID)
	depType := strings.TrimSpace(req.Type)
	if depType == "" {
		depType = string(models.DependencyBlocks)
	}

	if err := s.service.AddDependency(r.Context(), childID, parentID, depType); err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"child_id": childID, "parent_id": parentID, "type": depType})
}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListAllLabels(r.Context())
	if err != nil {
		s.writeErrorReq(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleListTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	labels, err := s.store.ListLabels(r.Context(), id)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleAddTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.LabelsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	labels, err := s.service.AddLabels(r.Context(), id, req.Labels)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.LabelsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	labels, err := s.service.RemoveLabels(r.Context(), id, req.Labels)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

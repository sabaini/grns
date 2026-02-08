package server

import (
	"net/http"
	"strings"

	"grns/internal/api"
	"grns/internal/models"
)

func (s *Server) taskLabelsRequest(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	if _, ok := s.pathProjectOrBadRequest(w, r); !ok {
		return "", nil, false
	}

	id, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return "", nil, false
	}

	var req api.LabelsRequest
	if !s.decodeJSONReq(w, r, &req) {
		return "", nil, false
	}

	return id, req.Labels, true
}

func (s *Server) handleDepTree(w http.ResponseWriter, r *http.Request) {
	project, ok := s.pathProjectOrBadRequest(w, r)
	if !ok {
		return
	}

	id, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	nodes, err := s.store.DependencyTree(r.Context(), project, id)
	if err != nil {
		s.writeStoreError(w, r, err)
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
	if _, ok := s.pathProjectOrBadRequest(w, r); !ok {
		return
	}

	var req api.DepCreateRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	childID := strings.TrimSpace(req.ChildID)
	parentID := strings.TrimSpace(req.ParentID)
	depType := strings.TrimSpace(req.Type)
	if depType == "" {
		depType = string(models.DependencyBlocks)
	}

	if err := s.service.AddDependency(r.Context(), childID, parentID, depType); err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"child_id": childID, "parent_id": parentID, "type": depType})
}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	project, ok := s.pathProjectOrBadRequest(w, r)
	if !ok {
		return
	}

	labels, err := s.store.ListAllLabels(r.Context(), project)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleListTaskLabels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.pathProjectOrBadRequest(w, r); !ok {
		return
	}

	id, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	labels, err := s.store.ListLabels(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleAddTaskLabels(w http.ResponseWriter, r *http.Request) {
	id, labelsReq, ok := s.taskLabelsRequest(w, r)
	if !ok {
		return
	}

	labels, err := s.service.AddLabels(r.Context(), id, labelsReq)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveTaskLabels(w http.ResponseWriter, r *http.Request) {
	id, labelsReq, ok := s.taskLabelsRequest(w, r)
	if !ok {
		return
	}

	labels, err := s.service.RemoveLabels(r.Context(), id, labelsReq)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

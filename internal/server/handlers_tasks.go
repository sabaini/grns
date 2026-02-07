package server

import (
	"fmt"
	"net/http"
	"time"

	"grns/internal/api"
)

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	var req api.TaskCloseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("ids are required"))
		return
	}
	for _, id := range req.IDs {
		if !validateID(id) {
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
			return
		}
	}

	if err := s.service.Close(r.Context(), req.IDs); err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	var req api.TaskReopenRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("ids are required"))
		return
	}
	for _, id := range req.IDs {
		if !validateID(id) {
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
			return
		}
	}

	if err := s.service.Reopen(r.Context(), req.IDs); err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit")
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	responses, err := s.service.Ready(r.Context(), limit)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleStale(w http.ResponseWriter, r *http.Request) {
	days, err := queryIntDefault(r, "days", 30)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	limit, err := queryInt(r, "limit")
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	statuses := splitCSV(r.URL.Query().Get("status"))

	if len(statuses) > 0 {
		normalized := make([]string, 0, len(statuses))
		for _, status := range statuses {
			value, err := normalizeStatus(status)
			if err != nil {
				s.writeErrorReq(w, r, http.StatusBadRequest, err)
				return
			}
			normalized = append(normalized, value)
		}
		statuses = normalized
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	responses, err := s.service.Stale(r.Context(), cutoff, statuses, limit)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req api.TaskCreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	resp, err := s.service.Create(r.Context(), req)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	var reqs []api.TaskCreateRequest
	if err := decodeJSON(w, r, &reqs); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	responses, err := s.service.BatchCreate(r.Context(), reqs)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusCreated, responses)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	resp, err := s.service.Get(r.Context(), id)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	var req api.TaskGetManyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("ids are required"))
		return
	}
	for _, id := range req.IDs {
		if !validateID(id) {
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
			return
		}
	}

	responses, err := s.service.GetMany(r.Context(), req.IDs)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.TaskUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	resp, err := s.service.Update(r.Context(), id, req)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	heavySearch := filter.SearchQuery != "" || filter.SpecRegex != ""
	if heavySearch {
		if !s.acquireLimiter(s.searchLimiter, w, "search") {
			return
		}
		defer s.releaseLimiter(s.searchLimiter)
	}

	responses, err := s.service.List(r.Context(), filter)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

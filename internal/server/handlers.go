package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/store"
)

const (
	defaultStatus   = "open"
	defaultType     = "task"
	defaultPriority = 2
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	var req api.TaskCloseRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("ids are required"))
		return
	}
	for _, id := range req.IDs {
		if !validateID(id) {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
			return
		}
	}

	if err := s.service.Close(r.Context(), req.IDs); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	var req api.TaskReopenRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.IDs) == 0 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("ids are required"))
		return
	}
	for _, id := range req.IDs {
		if !validateID(id) {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
			return
		}
	}

	if err := s.service.Reopen(r.Context(), req.IDs); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit")
	responses, err := s.service.Ready(r.Context(), limit)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleStale(w http.ResponseWriter, r *http.Request) {
	days := queryIntDefault(r, "days", 30)
	limit := queryInt(r, "limit")
	statuses := splitCSV(r.URL.Query().Get("status"))

	if len(statuses) > 0 {
		normalized := make([]string, 0, len(statuses))
		for _, status := range statuses {
			value, err := normalizeStatus(status)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err)
				return
			}
			normalized = append(normalized, value)
		}
		statuses = normalized
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	responses, err := s.service.Stale(r.Context(), cutoff, statuses, limit)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleDeps(w http.ResponseWriter, r *http.Request) {
	var req api.DepCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	childID := strings.TrimSpace(req.ChildID)
	parentID := strings.TrimSpace(req.ParentID)
	if !validateID(childID) || !validateID(parentID) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid dependency ids"))
		return
	}

	depType := strings.TrimSpace(req.Type)
	if depType == "" {
		depType = "blocks"
	}

	if err := s.store.AddDependency(r.Context(), childID, parentID, depType); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"child_id": childID, "parent_id": parentID, "type": depType})
}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListAllLabels(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req api.TaskCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.service.Create(r.Context(), req)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	var reqs []api.TaskCreateRequest
	if err := decodeJSON(r, &reqs); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	responses := make([]api.TaskResponse, 0, len(reqs))
	for _, req := range reqs {
		resp, err := s.service.Create(r.Context(), req)
		if err != nil {
			s.writeError(w, httpStatusFromError(err), err)
			return
		}
		responses = append(responses, resp)
	}

	s.writeJSON(w, http.StatusCreated, responses)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	resp, err := s.service.Get(r.Context(), id)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.TaskUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.service.Update(r.Context(), id, req)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	filter := store.ListFilter{
		Statuses:  splitCSV(r.URL.Query().Get("status")),
		Types:     splitCSV(r.URL.Query().Get("type")),
		ParentID:  strings.TrimSpace(r.URL.Query().Get("parent_id")),
		Labels:    splitCSV(r.URL.Query().Get("label")),
		LabelsAny: splitCSV(r.URL.Query().Get("label_any")),
		Limit:     queryInt(r, "limit"),
		Offset:    queryInt(r, "offset"),
	}

	if filter.ParentID != "" && !validateID(filter.ParentID) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid parent_id"))
		return
	}

	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			value, err := normalizeStatus(status)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err)
				return
			}
			statuses = append(statuses, value)
		}
		filter.Statuses = statuses
	}
	if len(filter.Types) > 0 {
		types := make([]string, 0, len(filter.Types))
		for _, t := range filter.Types {
			value, err := normalizeType(t)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err)
				return
			}
			types = append(types, value)
		}
		filter.Types = types
	}

	if len(filter.Labels) > 0 {
		labels, err := normalizeLabels(filter.Labels)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		filter.Labels = labels
	}
	if len(filter.LabelsAny) > 0 {
		labels, err := normalizeLabels(filter.LabelsAny)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		filter.LabelsAny = labels
	}

	if priority := r.URL.Query().Get("priority"); priority != "" {
		value, err := strconv.Atoi(priority)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid priority"))
			return
		}
		if value < 0 || value > 4 {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("priority must be between 0 and 4"))
			return
		}
		filter.Priority = &value
	}
	if priorityMin := r.URL.Query().Get("priority_min"); priorityMin != "" {
		value, err := strconv.Atoi(priorityMin)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid priority_min"))
			return
		}
		if value < 0 || value > 4 {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("priority_min must be between 0 and 4"))
			return
		}
		filter.PriorityMin = &value
	}
	if priorityMax := r.URL.Query().Get("priority_max"); priorityMax != "" {
		value, err := strconv.Atoi(priorityMax)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid priority_max"))
			return
		}
		if value < 0 || value > 4 {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("priority_max must be between 0 and 4"))
			return
		}
		filter.PriorityMax = &value
	}

	if filter.PriorityMin != nil && filter.PriorityMax != nil {
		if *filter.PriorityMin > *filter.PriorityMax {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("priority_min cannot be greater than priority_max"))
			return
		}
	}

	if assignee := strings.TrimSpace(r.URL.Query().Get("assignee")); assignee != "" {
		filter.Assignee = assignee
	}
	if r.URL.Query().Get("no_assignee") == "true" {
		filter.NoAssignee = true
	}
	if ids := splitCSV(r.URL.Query().Get("id")); len(ids) > 0 {
		filter.IDs = ids
	}
	if v := strings.TrimSpace(r.URL.Query().Get("title_contains")); v != "" {
		filter.TitleContains = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("desc_contains")); v != "" {
		filter.DescContains = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("notes_contains")); v != "" {
		filter.NotesContains = v
	}
	if v := r.URL.Query().Get("created_after"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid created_after: %w", err))
			return
		}
		filter.CreatedAfter = &t
	}
	if v := r.URL.Query().Get("created_before"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid created_before: %w", err))
			return
		}
		filter.CreatedBefore = &t
	}
	if v := r.URL.Query().Get("updated_after"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid updated_after: %w", err))
			return
		}
		filter.UpdatedAfter = &t
	}
	if v := r.URL.Query().Get("updated_before"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid updated_before: %w", err))
			return
		}
		filter.UpdatedBefore = &t
	}
	if v := r.URL.Query().Get("closed_after"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid closed_after: %w", err))
			return
		}
		filter.ClosedAfter = &t
	}
	if v := r.URL.Query().Get("closed_before"); v != "" {
		t, err := parseFlexibleTime(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid closed_before: %w", err))
			return
		}
		filter.ClosedBefore = &t
	}
	if r.URL.Query().Get("empty_description") == "true" {
		filter.EmptyDescription = true
	}
	if r.URL.Query().Get("no_labels") == "true" {
		filter.NoLabels = true
	}

	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	if spec != "" {
		pattern := "(?i)" + spec
		if _, err := regexp.Compile(pattern); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid spec regex"))
			return
		}
		filter.SpecRegex = pattern
	}

	responses, err := s.service.List(r.Context(), filter)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleListTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	labels, err := s.store.ListLabels(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleAddTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.LabelsRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	labels, err := normalizeLabels(req.Labels)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := s.store.AddLabels(r.Context(), id, labels); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	labels, err = s.store.ListLabels(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveTaskLabels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	var req api.LabelsRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	labels, err := normalizeLabels(req.Labels)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := s.store.RemoveLabels(r.Context(), id, labels); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	labels, err = s.store.ListLabels(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	code := errorCode(status, err)
	if status >= 500 {
		s.logger.Error("request error", "status", status, "code", code, "error", err)
	}
	s.writeJSON(w, status, api.ErrorResponse{Error: err.Error(), Code: code})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type apiError struct {
	status int
	code   string
	err    error
}

func (e apiError) Error() string {
	return e.err.Error()
}

func badRequest(err error) error {
	return apiError{status: http.StatusBadRequest, code: "invalid_argument", err: err}
}

func notFound(err error) error {
	return apiError{status: http.StatusNotFound, code: "not_found", err: err}
}

func conflict(err error) error {
	return apiError{status: http.StatusConflict, code: "conflict", err: err}
}

func httpStatusFromError(err error) int {
	if apiErr, ok := err.(apiError); ok {
		return apiErr.status
	}
	return http.StatusInternalServerError
}

func errorCode(status int, err error) string {
	if apiErr, ok := err.(apiError); ok && apiErr.code != "" {
		return apiErr.code
	}
	switch status {
	case http.StatusBadRequest:
		return "invalid_argument"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusInternalServerError:
		return "internal"
	default:
		return ""
	}
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: tasks.id")
}

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func queryInt(r *http.Request, key string) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func queryIntDefault(r *http.Request, key string, def int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return parsed
}

func valueOrEmpty(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return strings.TrimSpace(*ptr)
}

func parseFlexibleTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse("2006-01-02", value)
	if err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD format")
}

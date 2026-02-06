package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
	"grns/internal/store"
)

const (
	defaultStatus         = "open"
	defaultType           = "task"
	defaultPriority       = 2
	exportPageSize        = 500
	defaultJSONMaxBody    = 1 << 20  // 1 MiB
	batchJSONMaxBody      = 8 << 20  // 8 MiB
	importJSONMaxBody     = 64 << 20 // 64 MiB
	importStreamChunkSize = 500
	importStreamMaxLine   = 10 << 20 // 10 MiB
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.store.StoreInfo(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := api.InfoResponse{
		ProjectPrefix: s.projectPrefix,
		SchemaVersion: info.SchemaVersion,
		TaskCounts:    info.TaskCounts,
		TotalTasks:    info.TotalTasks,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDepTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateID(id) {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	nodes, err := s.store.DependencyTree(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
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

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	var req api.TaskCloseRequest
	if err := decodeJSON(w, r, &req); err != nil {
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
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	var req api.TaskReopenRequest
	if err := decodeJSON(w, r, &req); err != nil {
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
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"ids": req.IDs})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	responses, err := s.service.Ready(r.Context(), limit)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleStale(w http.ResponseWriter, r *http.Request) {
	days, err := queryIntDefault(r, "days", 30)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	limit, err := queryInt(r, "limit")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
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
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	childID := strings.TrimSpace(req.ChildID)
	parentID := strings.TrimSpace(req.ParentID)
	depType := strings.TrimSpace(req.Type)
	if depType == "" {
		depType = "blocks"
	}

	if err := s.service.AddDependency(r.Context(), childID, parentID, depType); err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"child_id": childID, "parent_id": parentID, "type": depType})
}

func (s *Server) handleAdminCleanup(w http.ResponseWriter, r *http.Request) {
	var req api.CleanupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.OlderThanDays <= 0 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("older_than_days must be > 0"))
		return
	}
	if !req.DryRun && r.Header.Get("X-Confirm") != "true" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("non-dry-run requires X-Confirm: true header"))
		return
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -req.OlderThanDays)
	result, err := s.store.CleanupClosedTasks(r.Context(), cutoff, req.DryRun)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := api.CleanupResponse{
		TaskIDs: result.TaskIDs,
		Count:   result.Count,
		DryRun:  result.DryRun,
	}
	if resp.TaskIDs == nil {
		resp.TaskIDs = []string{}
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.exportLimiter, w, "export") {
		return
	}
	defer s.releaseLimiter(s.exportLimiter)

	ctx := r.Context()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)

	offset := 0
	for {
		tasks, err := s.store.ListTasks(ctx, store.ListFilter{Limit: exportPageSize, Offset: offset})
		if err != nil {
			s.logger.Error("export list tasks", "error", err)
			return
		}
		if len(tasks) == 0 {
			return
		}

		ids := make([]string, 0, len(tasks))
		for _, t := range tasks {
			ids = append(ids, t.ID)
		}
		labelMap, err := s.store.ListLabelsForTasks(ctx, ids)
		if err != nil {
			s.logger.Error("export list labels", "error", err)
			return
		}
		depMap, err := s.store.ListDependenciesForTasks(ctx, ids)
		if err != nil {
			s.logger.Error("export list dependencies", "error", err)
			return
		}

		for _, t := range tasks {
			labels := labelMap[t.ID]
			if labels == nil {
				labels = []string{}
			}
			deps := depMap[t.ID]
			record := api.TaskResponse{Task: t, Labels: labels, Deps: deps}
			if err := enc.Encode(record); err != nil {
				s.logger.Error("export encode", "error", err)
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}

		offset += len(tasks)
	}
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.importLimiter, w, "import") {
		return
	}
	defer s.releaseLimiter(s.importLimiter)

	var req api.ImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// Validate dedupe.
	switch req.Dedupe {
	case "", "skip", "overwrite", "error":
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid dedupe mode: %s", req.Dedupe))
		return
	}
	// Validate orphan handling.
	switch req.OrphanHandling {
	case "", "allow", "skip", "strict":
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid orphan_handling: %s", req.OrphanHandling))
		return
	}

	if len(req.Tasks) == 0 {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("tasks array is required"))
		return
	}

	resp, err := s.service.Import(r.Context(), req)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleImportStream(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.importLimiter, w, "import") {
		return
	}
	defer s.releaseLimiter(s.importLimiter)

	dedupe := strings.TrimSpace(r.URL.Query().Get("dedupe"))
	orphanHandling := strings.TrimSpace(r.URL.Query().Get("orphan_handling"))
	dryRunValue := strings.TrimSpace(r.URL.Query().Get("dry_run"))
	dryRun := false
	if dryRunValue != "" {
		parsed, err := strconv.ParseBool(dryRunValue)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid dry_run"))
			return
		}
		dryRun = parsed
	}

	switch dedupe {
	case "", "skip", "overwrite", "error":
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid dedupe mode: %s", dedupe))
		return
	}
	switch orphanHandling {
	case "", "allow", "skip", "strict":
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid orphan_handling: %s", orphanHandling))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(importJSONMaxBody))
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), importStreamMaxLine)

	response := api.ImportResponse{DryRun: dryRun, TaskIDs: []string{}}
	chunk := make([]api.TaskImportRecord, 0, importStreamChunkSize)
	lineNum := 0
	hasRecords := false

	flushChunk := func() error {
		if len(chunk) == 0 {
			return nil
		}
		resp, err := s.service.Import(r.Context(), api.ImportRequest{
			Tasks:          chunk,
			DryRun:         dryRun,
			Dedupe:         dedupe,
			OrphanHandling: orphanHandling,
		})
		if err != nil {
			return err
		}
		response.Created += resp.Created
		response.Updated += resp.Updated
		response.Skipped += resp.Skipped
		response.Errors += resp.Errors
		response.TaskIDs = append(response.TaskIDs, resp.TaskIDs...)
		response.Messages = append(response.Messages, resp.Messages...)
		chunk = chunk[:0]
		return nil
	}

	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		hasRecords = true

		var rec api.TaskImportRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Errorf("line %d: %w", lineNum, err))
			return
		}
		chunk = append(chunk, rec)

		if len(chunk) >= importStreamChunkSize {
			if err := flushChunk(); err != nil {
				s.writeError(w, httpStatusFromError(err), err)
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("reading input: %w", err))
		return
	}
	if !hasRecords {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("no records found in input"))
		return
	}
	if err := flushChunk(); err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, response)
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
	if err := decodeJSON(w, r, &req); err != nil {
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
	if err := decodeJSON(w, r, &reqs); err != nil {
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
	if err := decodeJSON(w, r, &req); err != nil {
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
	limit, err := queryInt(r, "limit")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	offset, err := queryInt(r, "offset")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	filter := store.ListFilter{
		Statuses:  splitCSV(r.URL.Query().Get("status")),
		Types:     splitCSV(r.URL.Query().Get("type")),
		ParentID:  strings.TrimSpace(r.URL.Query().Get("parent_id")),
		Labels:    splitCSV(r.URL.Query().Get("label")),
		LabelsAny: splitCSV(r.URL.Query().Get("label_any")),
		Limit:     limit,
		Offset:    offset,
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
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		filter.SearchQuery = search
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

	heavySearch := filter.SearchQuery != "" || filter.SpecRegex != ""
	if heavySearch {
		if !s.acquireLimiter(s.searchLimiter, w, "search") {
			return
		}
		defer s.releaseLimiter(s.searchLimiter)
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
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	labels, err := s.service.AddLabels(r.Context(), id, req.Labels)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
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
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	labels, err := s.service.RemoveLabels(r.Context(), id, req.Labels)
	if err != nil {
		s.writeError(w, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, labels)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	code := errorCode(status, err)
	message := err.Error()
	if status >= 500 {
		s.logger.Error("request error", "status", status, "code", code, "error", err)
		message = "internal error"
	}
	s.writeJSON(w, status, api.ErrorResponse{Error: message, Code: code})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("write json response", "status", status, "error", err)
	}
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

func isInvalidSearchQuery(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unterminated string") ||
		strings.Contains(message, "fts5") && strings.Contains(message, "syntax") ||
		strings.Contains(message, "malformed match")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := defaultJSONMaxBody
	switch r.URL.Path {
	case "/v1/import":
		maxBytes = importJSONMaxBody
	case "/v1/tasks/batch":
		maxBytes = batchJSONMaxBody
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
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

func queryInt(r *http.Request, key string) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
	}
	return parsed, nil
}

func queryIntDefault(r *http.Request, key string, def int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
	}
	return parsed, nil
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

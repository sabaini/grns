package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"grns/internal/api"
	"grns/internal/store"
)

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
			s.logger.Error("export list tasks", "method", r.Method, "path", r.URL.Path, "offset", offset, "error", err)
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
			s.logger.Error("export list labels", "method", r.Method, "path", r.URL.Path, "offset", offset, "error", err)
			return
		}
		depMap, err := s.store.ListDependenciesForTasks(ctx, ids)
		if err != nil {
			s.logger.Error("export list dependencies", "method", r.Method, "path", r.URL.Path, "offset", offset, "error", err)
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
				s.logger.Error("export encode", "method", r.Method, "path", r.URL.Path, "task_id", t.ID, "error", err)
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
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate dedupe.
	switch req.Dedupe {
	case "", "skip", "overwrite", "error":
	default:
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid dedupe mode: %s", req.Dedupe))
		return
	}
	// Validate orphan handling.
	switch req.OrphanHandling {
	case "", "allow", "skip", "strict":
	default:
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid orphan_handling: %s", req.OrphanHandling))
		return
	}

	if len(req.Tasks) == 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("tasks array is required"))
		return
	}

	resp, err := s.service.Import(r.Context(), req)
	if err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
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
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid dry_run"))
			return
		}
		dryRun = parsed
	}
	atomicValue := strings.TrimSpace(r.URL.Query().Get("atomic"))
	atomic := false
	if atomicValue != "" {
		parsed, err := strconv.ParseBool(atomicValue)
		if err != nil {
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid atomic"))
			return
		}
		atomic = parsed
	}

	switch dedupe {
	case "", "skip", "overwrite", "error":
	default:
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid dedupe mode: %s", dedupe))
		return
	}
	switch orphanHandling {
	case "", "allow", "skip", "strict":
	default:
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("invalid orphan_handling: %s", orphanHandling))
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
			Atomic:         atomic,
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
		response.AppliedChunks += resp.AppliedChunks
		if response.ApplyMode == "" {
			response.ApplyMode = resp.ApplyMode
		}
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
			s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("line %d: %w", lineNum, err))
			return
		}
		chunk = append(chunk, rec)

		if len(chunk) >= importStreamChunkSize {
			if err := flushChunk(); err != nil {
				s.writeErrorReq(w, r, httpStatusFromError(err), err)
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("reading input: %w", err))
		return
	}
	if !hasRecords {
		s.writeErrorReq(w, r, http.StatusBadRequest, fmt.Errorf("no records found in input"))
		return
	}
	if err := flushChunk(); err != nil {
		s.writeErrorReq(w, r, httpStatusFromError(err), err)
		return
	}

	s.writeJSON(w, http.StatusOK, response)
}

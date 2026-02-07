package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"grns/internal/api"
)

type importStreamOptions struct {
	dedupe         string
	orphanHandling string
	dryRun         bool
	atomic         bool
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.exportLimiter, w, "export") {
		return
	}
	defer s.releaseLimiter(s.exportLimiter)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)

	offset := 0
	for {
		records, err := s.service.ExportPage(r.Context(), exportPageSize, offset)
		if err != nil {
			s.logExportError(r, "page", offset, "", err)
			return
		}
		if len(records) == 0 {
			return
		}

		for _, record := range records {
			if err := enc.Encode(record); err != nil {
				s.logExportError(r, "encode", offset, record.Task.ID, err)
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}

		offset += len(records)
	}
}

func (s *Server) logExportError(r *http.Request, stage string, offset int, taskID string, err error) {
	fields := []any{"stage", stage, "method", r.Method, "path", r.URL.Path, "offset", offset, "error", err}
	if taskID != "" {
		fields = append(fields, "task_id", taskID)
	}
	s.logger.Error("export failed", fields...)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.importLimiter, w, "import") {
		return
	}
	defer s.releaseLimiter(s.importLimiter)

	var req api.ImportRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	if err := validateImportModes(req.Dedupe, req.OrphanHandling); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	if len(req.Tasks) == 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("tasks array is required"), ErrCodeMissingRequired))
		return
	}

	resp, err := s.service.Import(r.Context(), req)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleImportStream(w http.ResponseWriter, r *http.Request) {
	if !s.acquireLimiter(s.importLimiter, w, "import") {
		return
	}
	defer s.releaseLimiter(s.importLimiter)

	opts, err := parseImportStreamOptions(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(importJSONMaxBody))
	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), importStreamMaxLine)

	response := api.ImportResponse{DryRun: opts.dryRun, TaskIDs: []string{}}
	chunk := make([]api.TaskImportRecord, 0, importStreamChunkSize)
	lineNum := 0
	hasRecords := false

	flushChunk := func() error {
		if len(chunk) == 0 {
			return nil
		}
		resp, err := s.service.Import(r.Context(), api.ImportRequest{
			Tasks:          chunk,
			DryRun:         opts.dryRun,
			Dedupe:         opts.dedupe,
			OrphanHandling: opts.orphanHandling,
			Atomic:         opts.atomic,
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
			s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("line %d: %w", lineNum, err), ErrCodeInvalidJSON))
			return
		}
		chunk = append(chunk, rec)

		if len(chunk) >= importStreamChunkSize {
			if err := flushChunk(); err != nil {
				s.writeServiceError(w, r, err)
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("reading input: %w", err), ErrCodeInvalidJSON))
		return
	}
	if !hasRecords {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("no records found in input"), ErrCodeMissingRequired))
		return
	}
	if err := flushChunk(); err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, response)
}

func parseImportStreamOptions(r *http.Request) (importStreamOptions, error) {
	opts := importStreamOptions{
		dedupe:         strings.TrimSpace(r.URL.Query().Get("dedupe")),
		orphanHandling: strings.TrimSpace(r.URL.Query().Get("orphan_handling")),
	}

	dryRun, err := queryBool(r, "dry_run")
	if err != nil {
		return importStreamOptions{}, err
	}
	opts.dryRun = dryRun

	atomic, err := queryBool(r, "atomic")
	if err != nil {
		return importStreamOptions{}, err
	}
	opts.atomic = atomic

	if err := validateImportModes(opts.dedupe, opts.orphanHandling); err != nil {
		return importStreamOptions{}, err
	}

	return opts, nil
}

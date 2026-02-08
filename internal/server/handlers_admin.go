package server

import (
	"fmt"
	"net/http"
	"time"

	"grns/internal/api"
)

func (s *Server) handleAdminCleanup(w http.ResponseWriter, r *http.Request) {
	var req api.CleanupRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}
	if req.OlderThanDays <= 0 {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("older_than_days must be > 0"), ErrCodeInvalidQuery))
		return
	}
	if !req.DryRun && r.Header.Get("X-Confirm") != "true" {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("non-dry-run requires X-Confirm: true header"), ErrCodeMissingRequired))
		return
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -req.OlderThanDays)
	result, err := s.store.CleanupClosedTasks(r.Context(), cutoff, req.DryRun)
	if err != nil {
		s.writeStoreError(w, r, err)
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

func (s *Server) handleAdminGCBlobs(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	var req api.BlobGCRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}
	if !req.DryRun && r.Header.Get("X-Confirm") != "true" {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("non-dry-run requires X-Confirm: true header"), ErrCodeMissingRequired))
		return
	}

	result, err := s.attachmentService.GCBlobs(r.Context(), req.BatchSize, !req.DryRun)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	resp := api.BlobGCResponse{
		CandidateCount: result.CandidateCount,
		DeletedCount:   result.DeletedCount,
		FailedCount:    result.FailedCount,
		ReclaimedBytes: result.ReclaimedBytes,
		DryRun:         result.DryRun,
	}
	s.writeJSON(w, http.StatusOK, resp)
}

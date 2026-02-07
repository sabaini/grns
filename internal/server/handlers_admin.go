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

package server

import (
	"net/http"

	"grns/internal/api"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.store.StoreInfo(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}

	resp := api.InfoResponse{
		DBPath:        s.dbPath,
		ProjectPrefix: s.projectPrefix,
		SchemaVersion: info.SchemaVersion,
		TaskCounts:    info.TaskCounts,
		TotalTasks:    info.TotalTasks,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

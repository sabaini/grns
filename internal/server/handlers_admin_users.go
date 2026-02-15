package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/store"
)

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		s.writeErrorReq(w, r, http.StatusNotImplemented, apiError{
			status:  http.StatusNotImplemented,
			code:    "not_implemented",
			errCode: ErrCodeNotImplemented,
			err:     fmt.Errorf("auth user provisioning is not supported"),
		})
		return
	}

	var req api.AdminUserCreateRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	created, err := s.authService.CreateAdminUser(r.Context(), req.Username, req.Password, time.Now().UTC())
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(err.Error()))
		switch {
		case isUniqueConstraint(err):
			s.writeErrorReq(w, r, http.StatusConflict, conflictCode(fmt.Errorf("username already exists"), ErrCodeConflict))
		case strings.Contains(message, "username") || strings.Contains(message, "password"):
			s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(err, ErrCodeInvalidArgument))
		default:
			s.writeStoreError(w, r, err)
		}
		return
	}

	s.writeJSON(w, http.StatusCreated, toAPIAdminUser(*created))
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		s.writeErrorReq(w, r, http.StatusNotImplemented, apiError{
			status:  http.StatusNotImplemented,
			code:    "not_implemented",
			errCode: ErrCodeNotImplemented,
			err:     fmt.Errorf("auth user provisioning is not supported"),
		})
		return
	}

	users, err := s.authService.ListUsers(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}

	resp := make([]api.AdminUser, 0, len(users))
	for _, user := range users {
		resp = append(resp, toAPIAdminUser(user))
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminSetUserDisabled(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		s.writeErrorReq(w, r, http.StatusNotImplemented, apiError{
			status:  http.StatusNotImplemented,
			code:    "not_implemented",
			errCode: ErrCodeNotImplemented,
			err:     fmt.Errorf("auth user provisioning is not supported"),
		})
		return
	}

	username, err := pathUsername(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	var req api.AdminUserSetDisabledRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	updated, err := s.authService.SetUserDisabled(r.Context(), username, req.Disabled, time.Now().UTC())
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(message, "username") {
			s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(err, ErrCodeInvalidArgument))
			return
		}
		s.writeStoreError(w, r, err)
		return
	}
	if updated == nil {
		s.writeErrorReq(w, r, http.StatusNotFound, notFoundCode(fmt.Errorf("user not found"), ErrCodeUserNotFound))
		return
	}

	s.writeJSON(w, http.StatusOK, toAPIAdminUser(*updated))
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		s.writeErrorReq(w, r, http.StatusNotImplemented, apiError{
			status:  http.StatusNotImplemented,
			code:    "not_implemented",
			errCode: ErrCodeNotImplemented,
			err:     fmt.Errorf("auth user provisioning is not supported"),
		})
		return
	}

	username, err := pathUsername(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	deleted, err := s.authService.DeleteUser(r.Context(), username)
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(message, "username") {
			s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(err, ErrCodeInvalidArgument))
			return
		}
		s.writeStoreError(w, r, err)
		return
	}
	if !deleted {
		s.writeErrorReq(w, r, http.StatusNotFound, notFoundCode(fmt.Errorf("user not found"), ErrCodeUserNotFound))
		return
	}

	s.writeJSON(w, http.StatusOK, api.AdminUserDeleteResponse{Username: strings.ToLower(strings.TrimSpace(username)), Deleted: true})
}

func pathUsername(r *http.Request) (string, error) {
	username := strings.TrimSpace(r.PathValue("username"))
	if username == "" {
		return "", badRequestCode(fmt.Errorf("username is required"), ErrCodeMissingRequired)
	}
	return username, nil
}

func toAPIAdminUser(user store.AuthUser) api.AdminUser {
	return api.AdminUser{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Disabled:  user.Disabled,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

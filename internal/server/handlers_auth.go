package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"grns/internal/api"
)

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		s.writeErrorReq(w, r, http.StatusNotImplemented, apiError{
			status:  http.StatusNotImplemented,
			code:    "not_implemented",
			errCode: ErrCodeNotImplemented,
			err:     fmt.Errorf("auth login not supported"),
		})
		return
	}

	var req api.AuthLoginRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	result, err := s.authService.Login(r.Context(), req.Username, req.Password, time.Now().UTC())
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(err.Error()))
		switch {
		case message == "invalid credentials":
			s.writeErrorReq(w, r, http.StatusUnauthorized, apiError{
				status:  http.StatusUnauthorized,
				code:    "unauthorized",
				errCode: ErrCodeUnauthorized,
				err:     fmt.Errorf("invalid credentials"),
			})
			return
		case strings.Contains(message, "username") || strings.Contains(message, "password"):
			s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(err, ErrCodeInvalidArgument))
			return
		default:
			s.writeStoreError(w, r, err)
			return
		}
	}

	ttlSeconds := int(defaultSessionTTL / time.Second)
	if ttlSeconds <= 0 {
		ttlSeconds = 86400
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    result.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   ttlSeconds,
		Expires:  result.ExpiresAt,
	})

	s.writeJSON(w, http.StatusOK, api.AuthMeResponse{
		Authenticated: true,
		AuthRequired:  true,
		Username:      result.User.Username,
		Role:          result.User.Role,
		AuthType:      authTypeSession,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	token := sessionTokenFromRequest(r)
	if token != "" && s.authService != nil {
		if err := s.authService.RevokeSessionToken(r.Context(), token, time.Now().UTC()); err != nil {
			s.writeStoreError(w, r, err)
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	requireAuth, err := s.apiAuthRequired(r)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	if !requireAuth {
		s.writeJSON(w, http.StatusOK, api.AuthMeResponse{
			Authenticated: false,
			AuthRequired:  false,
		})
		return
	}

	principal, ok := authPrincipalFromContext(r.Context())
	if !ok {
		s.writeErrorReq(w, r, http.StatusUnauthorized, apiError{
			status:  http.StatusUnauthorized,
			code:    "unauthorized",
			errCode: ErrCodeUnauthorized,
			err:     fmt.Errorf("unauthorized"),
		})
		return
	}

	resp := api.AuthMeResponse{
		Authenticated: true,
		AuthRequired:  true,
		AuthType:      principal.AuthType,
	}
	if principal.User != nil {
		resp.Username = principal.User.Username
		resp.Role = principal.User.Role
	}

	s.writeJSON(w, http.StatusOK, resp)
}

package server

import (
	"errors"
	"fmt"
	"net"
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

	now := time.Now().UTC()
	limiterKey := loginAttemptKey(req.Username, r)
	if s.loginLimiter != nil && !s.loginLimiter.Allow(limiterKey, now) {
		s.writeErrorReq(w, r, http.StatusTooManyRequests, apiError{
			status:  http.StatusTooManyRequests,
			code:    "resource_exhausted",
			errCode: ErrCodeResourceExhausted,
			err:     fmt.Errorf("too many login attempts; retry later"),
		})
		return
	}

	result, err := s.authService.Login(r.Context(), req.Username, req.Password, now)
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(err.Error()))
		switch {
		case errors.Is(err, errInvalidCredentials):
			if s.loginLimiter != nil {
				s.loginLimiter.RegisterFailure(limiterKey, now)
			}
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
	if s.loginLimiter != nil {
		s.loginLimiter.Reset(limiterKey)
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
	requireAuth, ok := authRequiredFromContext(r.Context())
	if !ok {
		computed, err := s.apiAuthRequired(r)
		if err != nil {
			s.writeStoreError(w, r, err)
			return
		}
		requireAuth = computed
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

func loginAttemptKey(username string, r *http.Request) string {
	user := strings.ToLower(strings.TrimSpace(username))
	if user == "" {
		user = "<empty>"
	}
	ip := requestClientIP(r)
	if ip == "" {
		ip = "<unknown>"
	}
	return ip + "|" + user
}

func requestClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remote)
	if err == nil {
		return strings.TrimSpace(host)
	}
	return remote
}

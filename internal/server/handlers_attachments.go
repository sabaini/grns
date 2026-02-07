package server

import (
	"bufio"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"grns/internal/api"
	"grns/internal/models"
)

const (
	attachmentUploadMaxBody   = 100 << 20 // 100 MiB
	attachmentMultipartMemory = 8 << 20   // 8 MiB
)

func (s *Server) handleCreateTaskAttachment(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	taskID, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(attachmentUploadMaxBody))
	if err := r.ParseMultipartForm(attachmentMultipartMemory); err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, classifyMultipartError(err))
		return
	}

	file, header, err := r.FormFile("content")
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("content is required"), ErrCodeMissingRequired))
		return
	}
	defer file.Close()

	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		s.writeErrorReq(w, r, http.StatusBadRequest, badRequestCode(fmt.Errorf("kind is required"), ErrCodeMissingRequired))
		return
	}

	expiresAt, err := parseOptionalExpiresAt(r.FormValue("expires_at"))
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	labels := parseAttachmentFormLabels(r.MultipartForm)

	buffered := bufio.NewReader(file)
	peek, _ := buffered.Peek(512)
	detectedMediaType := http.DetectContentType(peek)
	declaredMediaType := strings.TrimSpace(r.FormValue("media_type"))
	mediaType := declaredMediaType
	mediaTypeSource := ""
	if mediaType == "" {
		mediaType = detectedMediaType
		mediaTypeSource = "sniffed"
	}

	attachment, err := s.attachmentService.CreateManagedAttachmentFromReader(r.Context(), taskID, CreateManagedAttachmentInput{
		Kind:            kind,
		Title:           strings.TrimSpace(r.FormValue("title")),
		Filename:        firstNonEmpty(strings.TrimSpace(r.FormValue("filename")), header.Filename),
		MediaType:       mediaType,
		MediaTypeSource: mediaTypeSource,
		Labels:          labels,
		Meta:            map[string]any{},
		ExpiresAt:       expiresAt,
	}, buffered)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusCreated, attachment)
}

func (s *Server) handleCreateTaskAttachmentLink(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	taskID, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	var req api.AttachmentCreateLinkRequest
	if !s.decodeJSONReq(w, r, &req) {
		return
	}

	attachment, err := s.attachmentService.CreateLinkAttachment(r.Context(), taskID, CreateLinkAttachmentInput{
		Kind:        req.Kind,
		Title:       req.Title,
		Filename:    req.Filename,
		MediaType:   req.MediaType,
		ExternalURL: req.ExternalURL,
		RepoPath:    req.RepoPath,
		Labels:      req.Labels,
		Meta:        req.Meta,
		ExpiresAt:   req.ExpiresAt,
	})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusCreated, attachment)
}

func (s *Server) handleListTaskAttachments(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	taskID, ok := s.pathIDOrBadRequest(w, r)
	if !ok {
		return
	}

	attachments, err := s.attachmentService.ListTaskAttachments(r.Context(), taskID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if attachments == nil {
		attachments = []models.Attachment{}
	}

	s.writeJSON(w, http.StatusOK, attachments)
}

func (s *Server) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	attachmentID, err := requireAttachmentID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}

	attachment, err := s.attachmentService.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, attachment)
}

func (s *Server) handleGetAttachmentContent(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	attachmentID, err := requireAttachmentID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	if err := s.attachmentService.StreamAttachmentContent(r.Context(), attachmentID); err != nil {
		s.writeServiceError(w, r, notImplemented(err))
		return
	}
}

func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	if s.attachmentService == nil {
		s.writeServiceError(w, r, internalError(fmt.Errorf("attachments are not configured")))
		return
	}

	attachmentID, err := requireAttachmentID(r)
	if err != nil {
		s.writeErrorReq(w, r, http.StatusBadRequest, err)
		return
	}
	if err := s.attachmentService.DeleteAttachment(r.Context(), attachmentID); err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"id": attachmentID})
}

func requireAttachmentID(r *http.Request) (string, error) {
	id := strings.TrimSpace(r.PathValue("attachment_id"))
	if !validateID(id) {
		return "", badRequestCode(fmt.Errorf("invalid attachment_id"), ErrCodeInvalidID)
	}
	return id, nil
}

func parseOptionalExpiresAt(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseFlexibleTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseAttachmentFormLabels(form *multipart.Form) []string {
	if form == nil || form.Value == nil {
		return nil
	}
	values := make([]string, 0)
	for _, key := range []string{"label", "labels", "labels[]"} {
		for _, raw := range form.Value[key] {
			values = append(values, splitCSV(raw)...)
		}
	}
	return values
}

func classifyMultipartError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
		return badRequestCode(fmt.Errorf("request body too large"), ErrCodeRequestTooLarge)
	}
	return badRequestCode(err, ErrCodeInvalidArgument)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

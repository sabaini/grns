package server

const (
	// Validation (1xxx)
	ErrCodeInvalidArgument    = 1000
	ErrCodeInvalidJSON        = 1001
	ErrCodeRequestTooLarge    = 1002
	ErrCodeInvalidQuery       = 1003
	ErrCodeInvalidID          = 1004
	ErrCodeInvalidStatus      = 1005
	ErrCodeInvalidType        = 1006
	ErrCodeInvalidPriority    = 1007
	ErrCodeInvalidLabel       = 1008
	ErrCodeMissingRequired    = 1009
	ErrCodeInvalidTimeFilter  = 1010
	ErrCodeInvalidImportMode  = 1011
	ErrCodeInvalidDependency  = 1012
	ErrCodeInvalidParentID    = 1013
	ErrCodeInvalidSearchQuery = 1014

	// Domain state (2xxx)
	ErrCodeTaskNotFound       = 2001
	ErrCodeDependencyNotFound = 2002
	ErrCodeAttachmentNotFound = 2003
	ErrCodeGitRefNotFound     = 2004
	ErrCodeTaskIDExists       = 2101
	ErrCodeConflict           = 2102

	// Auth & limits (3xxx)
	ErrCodeUnauthorized      = 3001
	ErrCodeForbidden         = 3002
	ErrCodeResourceExhausted = 3003

	// Internal/system (4xxx)
	ErrCodeInternal       = 4001
	ErrCodeStoreFailure   = 4002
	ErrCodeExportFailed   = 4003
	ErrCodeImportFailed   = 4004
	ErrCodeNotImplemented = 4005
)

func defaultErrorCodeByStatus(status int) int {
	switch status {
	case 400:
		return ErrCodeInvalidArgument
	case 401:
		return ErrCodeUnauthorized
	case 403:
		return ErrCodeForbidden
	case 404:
		return ErrCodeTaskNotFound
	case 409:
		return ErrCodeConflict
	case 429:
		return ErrCodeResourceExhausted
	case 500:
		return ErrCodeInternal
	case 501:
		return ErrCodeNotImplemented
	default:
		return 0
	}
}

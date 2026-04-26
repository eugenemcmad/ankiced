package httpapi

import (
	"errors"
	"net/http"

	"ankiced/internal/application"
	"ankiced/internal/domain"
	"ankiced/internal/presentation"
)

type apiErrorPayload struct {
	Message           string `json:"message"`
	RecommendedAction string `json:"recommended_action"`
	Code              string `json:"code"`
	Details           string `json:"details"`
	CorrelationID     string `json:"correlation_id"`
}

func (h *apiHandler) writeError(w http.ResponseWriter, status int, err error, operation, cid string) {
	h.log("error", operation, cid, "request failed", map[string]any{"error": presentation.FormatDebugError(err)})
	code := mapErrorCode(err)
	h.writeAPIError(w, status, presentation.FormatError(err), code, presentation.FormatDebugError(err), cid)
}

func (h *apiHandler) writeMessageError(w http.ResponseWriter, status int, message, code, details, operation, cid string) {
	h.log("error", operation, cid, "request failed", map[string]any{"error": details})
	h.writeAPIError(w, status, message, code, details, cid)
}

// writeDecodeError translates errors from decodeJSON into the appropriate
// API error contract entry. It distinguishes "body too large" (raised by
// http.MaxBytesReader) from generic JSON parsing failures so clients can
// react to each case differently.
func (h *apiHandler) writeDecodeError(w http.ResponseWriter, err error, operation string) {
	cid := h.correlationID()
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		h.writeMessageError(w, http.StatusRequestEntityTooLarge, "request body too large", "REQUEST_BODY_TOO_LARGE", err.Error(), operation, cid)
		return
	}
	h.writeMessageError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", err.Error(), operation, cid)
}

func (h *apiHandler) writeMethodNotAllowed(w http.ResponseWriter, operation string) {
	cid := h.correlationID()
	h.writeMessageError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED", "request method is not supported for this endpoint", operation, cid)
}

func mapErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrEmptyDeckName):
		return "DECK_NAME_EMPTY"
	case errors.Is(err, domain.ErrDeckNameConflict):
		return "DECK_NAME_CONFLICT"
	case errors.Is(err, domain.ErrDeckNameTooLong):
		return "DECK_NAME_TOO_LONG"
	case errors.Is(err, domain.ErrDeckNameInvalid):
		return "DECK_NAME_INVALID"
	case errors.Is(err, application.ErrDeckNotFound):
		return "DECK_NOT_FOUND"
	case errors.Is(err, application.ErrNoteNotFound):
		return "NOTE_NOT_FOUND"
	case errors.Is(err, application.ErrModelNotFound):
		return "MODEL_NOT_FOUND"
	case errors.Is(err, domain.ErrDeckSearchEmpty):
		return "DECK_SEARCH_EMPTY"
	case errors.Is(err, domain.ErrInvalidNoteListFilters):
		return "NOTE_LIST_FILTERS_INVALID"
	case errors.Is(err, domain.ErrInvalidNoteID):
		return "NOTE_ID_INVALID"
	case errors.Is(err, domain.ErrFieldCountInvalid):
		return "NOTE_FIELD_COUNT_INVALID"
	case errors.Is(err, application.ErrTemplateNotFound):
		return "TEMPLATE_NOT_FOUND"
	default:
		return "INTERNAL_ERROR"
	}
}

func (h *apiHandler) writeAPIError(w http.ResponseWriter, status int, message, code, details, cid string) {
	h.writeJSON(w, status, map[string]any{
		"message":            message,
		"recommended_action": recommendedAction(code),
		"code":               code,
		"details":            details,
		"correlation_id":     cid,
	})
}

// recommendedActions maps API error codes to user-facing remediation hints.
// Codes in the same logical category (e.g. all deck-name validation issues)
// share the same hint to keep the UX consistent.
var recommendedActions = map[string]string{
	"DECK_NAME_EMPTY":          "Use a non-empty deck name without control characters and keep it reasonably short.",
	"DECK_NAME_INVALID":        "Use a non-empty deck name without control characters and keep it reasonably short.",
	"DECK_NAME_TOO_LONG":       "Use a non-empty deck name without control characters and keep it reasonably short.",
	"DECK_NAME_CONFLICT":       "Choose a deck name that is not already used by another deck.",
	"DECK_NOT_FOUND":           "Refresh the deck list and choose an existing deck.",
	"NOTE_NOT_FOUND":           "Refresh the note list and select an existing note id.",
	"MODEL_NOT_FOUND":          "Verify the note model still exists in the Anki collection.",
	"DECK_SEARCH_EMPTY":        "Enter search text before running deck search.",
	"NOTE_LIST_FILTERS_INVALID": "Select a deck, enter a note id, or provide global search text.",
	"NOTE_ID_INVALID":          "Enter a positive numeric note id.",
	"NOTE_FIELD_COUNT_INVALID": "Reload the note and save all fields returned by the editor.",
	"TEMPLATE_NOT_FOUND":       "Use an available action template or omit template_id to use the default cleaner.",
	"INVALID_JSON":             "Check the request body format and use valid JSON.",
	"DECK_ID_INVALID":          "Enter a positive numeric deck id.",
	"CONFIRM_REQUIRED":         "Review the dry run result and explicitly confirm apply.",
	"OPERATION_ID_INVALID":     "Check the operation id returned by the cleaner request.",
	"OPERATION_NOT_FOUND":      "Check the operation id returned by the cleaner request.",
	"METHOD_NOT_ALLOWED":       "Use the HTTP method documented for this endpoint.",
	"NOT_FOUND":                "Check the API path and version.",
	"APP_EXIT_DISABLED":        "Run ankiced-web through the normal application entrypoint to enable exit.",
	"STREAMING_UNSUPPORTED":    "Use a client that supports HTTP/1.1 streaming or fall back to the polling logs endpoint (?since=).",
	"REQUEST_BODY_TOO_LARGE":   "Reduce the size of the request body or split it into smaller chunks.",
}

const defaultRecommendedAction = "Check the technical details and retry the operation."

func recommendedAction(code string) string {
	if action, ok := recommendedActions[code]; ok {
		return action
	}
	return defaultRecommendedAction
}

package main

// Every route that can change data must appear in auditedRoutes (with the
// actions it writes) or in unauditedRoutes (with the reason nothing changes).
// TestEveryMutatingRouteDeclaresAuditCoverage fails for anything else, so a new
// route cannot ship without an explicit decision here.
//
// Keys are "METHOD /path" exactly as registered in newAPIRouter.
var auditedRoutes = map[string][]string{
	"POST /api/v1/directory-users":         {"DIRECTORY_USER_CREATED"},
	"POST /api/v1/documents":               {"DOCUMENT_UPLOADED", "DOCUMENT_OVERWRITTEN"},
	"POST /api/v1/documents/:id/file":      {"DOCUMENT_FILE_REPLACED"},
	"POST /api/v1/documents/:id/reprocess": {"DOCUMENT_RESCAN_REQUESTED", "DOCUMENT_REPROCESS_REQUESTED"},
	"PATCH /api/v1/documents/:id":          {"DOCUMENT_UPDATED"},
	"DELETE /api/v1/documents/:id":         {"DOCUMENT_DELETED"},
	"POST /api/v1/documents/:id/cancel":    {"DOCUMENT_CANCELLED"},
	"POST /api/v1/documents/:id/confirm":   {"DOCUMENT_CONFIRMED"},
	"POST /api/v1/sync/users":              {"DIRECTORY_SYNCED"},
	"PUT /api/v1/settings/ocr":             {"SETTINGS_OCR_UPDATED"},
	"PUT /api/v1/settings/ocr/api-key":     {"OCR_API_KEY_UPDATED"},
	"DELETE /api/v1/settings/ocr/api-key":  {"OCR_API_KEY_REMOVED"},
	"POST /api/v1/settings/drive/sync":     {"DRIVE_SYNC_STARTED"},
	"PUT /api/v1/me/photo":                 {"PROFILE_PHOTO_UPDATED"},
	"DELETE /api/v1/me/photo":              {"PROFILE_PHOTO_REMOVED"},
}

var unauditedRoutes = map[string]string{
	"POST /api/v1/settings/ocr/test":   "only calls the OCR API to check the connection; writes nothing",
	"POST /api/v1/settings/drive/test": "only checks that the Drive folder is reachable; writes nothing",
}

// auditedJobs lists the background jobs that change documents or directory
// data. Their audit rows use UserID "system:<job>".
var auditedJobs = map[string][]string{
	"document-queue":      {"DOCUMENT_PROCESSED", "DOCUMENT_EXTRACTION_FAILED"},
	"folder-import":       {"DOCUMENT_FOLDER_IMPORTED"},
	"google-drive-import": {"DOCUMENT_DRIVE_IMPORTED", "DRIVE_SYNC_PAUSED", "DRIVE_SYNC_FINISHED"},
	"user-sync":           {"DIRECTORY_SYNCED"},
	"expiry-notice":       {"EXPIRY_NOTICE_CREATED", "EXPIRY_EMAIL_SENT", "EXPIRY_EMAIL_SKIPPED"},
}

// auditSystemUser is the UserID of an audit row written by a background job.
func auditSystemUser(job string) string {
	return "system:" + job
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const driveFileFields = "id,name,mimeType,createdTime,modifiedTime,size,webViewLink,resourceKey,driveId,trashed,capabilities(canDownload,canEdit),shortcutDetails(targetId,targetMimeType,targetResourceKey)"
const driveFolderMime = "application/vnd.google-apps.folder"
const driveShortcutMime = "application/vnd.google-apps.shortcut"

// Each folder page and file has a durable checkpoint. Replaying a page after
// a crash cannot create duplicate tasks or documents.
type DriveSyncRun struct {
	ID              uint64     `json:"id" gorm:"primaryKey"`
	RootID          string     `json:"rootId"`
	RootResourceKey string     `json:"-"`
	FolderName      string     `json:"folderName"`
	Status          string     `json:"status"`
	TriggerType     string     `json:"triggerType"`
	StartedAt       time.Time  `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt"`
	RetryAt         *time.Time `json:"retryAt"`
	LastError       string     `json:"lastError"`
}

type DriveSyncFolder struct {
	RunID       uint64 `gorm:"primaryKey"`
	FolderID    string `gorm:"primaryKey"`
	ResourceKey string
	DriveID     string
	PageToken   string
	Status      string
	Attempts    int
	RetryAt     *time.Time
	LastError   string
}

type DriveSyncItem struct {
	RunID     uint64     `gorm:"primaryKey"`
	FileID    string     `gorm:"primaryKey"`
	FileName  string     `json:"fileName"`
	Metadata  string     `json:"-"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	RetryAt   *time.Time `json:"retryAt"`
	LastError string     `json:"lastError"`
}

type DriveImportedFile struct {
	FileID      string `gorm:"primaryKey"`
	DocumentID  uint64
	FileHash    string
	WebViewLink string `gorm:"size:2048"`
	ImportedAt  time.Time
}

func validDriveID(id string) bool {
	if id == "" || len(id) > 255 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// The last scheduled occurrence is calculated in the configured IANA timezone,
// including when the server restarts after the scheduled time.
func driveScheduledOccurrence(now time.Time, settings OCRSettings) time.Time {
	location, err := time.LoadLocation(settings.DriveTimezone)
	if err != nil {
		location = time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	clock, err := time.Parse("15:04", settings.DriveSyncTime)
	if err != nil {
		clock, _ = time.Parse("15:04", "05:00")
	}
	local := now.In(location)
	due := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, location)
	if due.After(local) {
		due = due.AddDate(0, 0, -1)
	}
	return due
}

func driveResourceHeader(header http.Header, id, key string) {
	if key != "" {
		header.Set("X-Goog-Drive-Resource-Keys", id+"/"+key)
	}
}

// Transient quota and server errors receive exponential backoff with jitter.
// Permanent errors stay visible in the durable queue and are retried next run.
func driveRetryable(err error) bool {
	var apiError *googleapi.Error
	if errors.As(err, &apiError) {
		if apiError.Code == 429 || apiError.Code >= 500 {
			return true
		}
		if apiError.Code == 403 {
			for _, detail := range apiError.Errors {
				if detail.Reason == "rateLimitExceeded" || detail.Reason == "userRateLimitExceeded" {
					return true
				}
			}
		}
		return false
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func driveRetry[T any](ctx context.Context, call func() (T, error)) (T, error) {
	var value T
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		value, err = call()
		if err == nil || !driveRetryable(err) {
			return value, err
		}
		if attempt == 4 {
			break
		}
		timer := time.NewTimer(time.Duration(1<<attempt)*time.Second + time.Duration(rand.Intn(1000))*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return value, ctx.Err()
		case <-timer.C:
		}
	}
	return value, err
}

// MySQL's connection-scoped lock also prevents simultaneous downloads when
// more than one backend instance is running. Keep the connection pinned.
func (i *GoogleDriveImporter) acquireLock(ctx context.Context) (*sql.Conn, error) {
	pool, err := i.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('docflow:google-drive', 0)").Scan(&locked); err != nil {
		conn.Close()
		return nil, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		conn.Close()
		return nil, errDriveBusy
	}
	return conn, nil
}

var errDriveBusy = errors.New("มีงานดึง Google Drive กำลังทำงานอยู่")

func releaseDriveLock(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, "SELECT RELEASE_LOCK('docflow:google-drive')")
	_ = conn.Close()
}

func (i *GoogleDriveImporter) prepareRun(ctx context.Context, manual bool) (*DriveSyncRun, error) {
	settings, err := i.settings.LoadOCR()
	if err != nil {
		return nil, err
	}
	if settings.ImportSource != "drive" {
		if manual {
			return nil, fmt.Errorf("กรุณาเลือก Google Drive เป็นแหล่งนำเข้าและบันทึกก่อน")
		}
		return nil, nil
	}
	rootID, err := i.configuredFolderID(settings.DriveFolderURL)
	if err != nil {
		return nil, err
	}
	resourceKey := ""
	if parsed, err := url.Parse(settings.DriveFolderURL); err == nil {
		resourceKey = parsed.Query().Get("resourcekey")
	}
	var latest DriveSyncRun
	err = i.db.WithContext(ctx).Where("root_id = ?", rootID).Order("id DESC").Take(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && latest.Status == "RUNNING" {
		if latest.RootResourceKey != resourceKey {
			if err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&latest).Updates(map[string]any{"root_resource_key": resourceKey, "retry_at": nil}).Error; err != nil {
					return err
				}
				return tx.Model(&DriveSyncFolder{}).Where("run_id = ? AND folder_id = ?", latest.ID, rootID).Update("resource_key", resourceKey).Error
			}); err != nil {
				return nil, err
			}
			latest.RootResourceKey = resourceKey
			latest.RetryAt = nil
		}
		if !manual && !settings.ImportEnabled && latest.TriggerType != "MANUAL" {
			return nil, nil
		}
		if manual && latest.TriggerType != "MANUAL" {
			if err := i.db.WithContext(ctx).Model(&latest).Update("trigger_type", "MANUAL").Error; err != nil {
				return nil, err
			}
			latest.TriggerType = "MANUAL"
		}
		if !manual && latest.RetryAt != nil && latest.RetryAt.After(time.Now()) {
			return nil, nil
		}
		return &latest, nil
	}
	if !manual {
		if !settings.ImportEnabled {
			return nil, nil
		}
		// A new source waits for today's scheduled time. Existing sources catch
		// up a missed run after a restart, and unfinished runs resume immediately.
		due := driveScheduledOccurrence(time.Now(), settings)
		if latest.ID == 0 && due.In(due.Location()).Format("2006-01-02") != time.Now().In(due.Location()).Format("2006-01-02") {
			return nil, nil
		}
		if latest.ID != 0 && !latest.StartedAt.Before(due) {
			return nil, nil
		}
	}
	trigger := "SCHEDULED"
	if manual {
		trigger = "MANUAL"
	}
	run := DriveSyncRun{RootID: rootID, RootResourceKey: resourceKey, Status: "RUNNING", TriggerType: trigger, StartedAt: time.Now()}
	err = i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Create(&DriveSyncFolder{RunID: run.ID, FolderID: rootID, ResourceKey: resourceKey, Status: "PENDING"}).Error
	})
	return &run, err
}

// Preserve run summaries and imported file IDs indefinitely, but bound the
// large per-file/per-folder snapshots to the ten latest runs per source.
func (i *GoogleDriveImporter) pruneSnapshots(ctx context.Context, rootID string) error {
	var boundary DriveSyncRun
	err := i.db.WithContext(ctx).Where("root_id = ?", rootID).Order("id DESC").Offset(9).Take(&boundary).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, table := range []string{"drive_sync_items", "drive_sync_folders"} {
		for ctx.Err() == nil {
			result := i.db.WithContext(ctx).Exec("DELETE FROM "+table+" WHERE run_id IN (SELECT id FROM drive_sync_runs WHERE root_id = ? AND id < ? AND status <> 'RUNNING') LIMIT 10000", rootID, boundary.ID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected < 10000 {
				break
			}
		}
	}
	return ctx.Err()
}

// StartNow acknowledges only after the job is committed. Processing does not
// depend on the HTTP request remaining open; the next tick resumes after a crash.
func (i *GoogleDriveImporter) StartNow() (*DriveSyncRun, error) {
	if !i.syncMu.TryLock() {
		return nil, errDriveBusy
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := i.acquireLock(ctx)
	if err != nil {
		i.syncMu.Unlock()
		return nil, err
	}
	run, err := i.prepareRun(ctx, true)
	if err != nil {
		releaseDriveLock(conn)
		i.syncMu.Unlock()
		return nil, err
	}
	response := *run
	go func() {
		defer i.syncMu.Unlock()
		defer releaseDriveLock(conn)
		workCtx, stop := context.WithTimeout(context.Background(), 10*time.Minute)
		defer stop()
		if _, err := i.processRun(workCtx, conn, run, true); err != nil {
			i.logger.Printf("Google Drive run %d: %v", run.ID, err)
		}
	}()
	return &response, nil
}

func (i *GoogleDriveImporter) SyncIfDue(ctx context.Context) (int, error) {
	if !i.syncMu.TryLock() {
		return 0, nil
	}
	defer i.syncMu.Unlock()
	conn, err := i.acquireLock(ctx)
	if errors.Is(err, errDriveBusy) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer releaseDriveLock(conn)
	run, err := i.prepareRun(ctx, false)
	if err != nil || run == nil {
		return 0, err
	}
	return i.processRun(ctx, conn, run, run.TriggerType == "MANUAL")
}

func (i *GoogleDriveImporter) processRun(ctx context.Context, conn *sql.Conn, run *DriveSyncRun, manual bool) (int, error) {
	imported := 0
	if err := i.pruneSnapshots(ctx, run.RootID); err != nil {
		return 0, err
	}
	defer func() {
		if ctx.Err() != nil {
			i.logger.Printf("Google Drive run %d checkpointed; continuing next tick", run.ID)
		}
	}()
	folder, err := i.getDriveFile(ctx, run.RootID, run.RootResourceKey)
	if err != nil || folder.MimeType != driveFolderMime || folder.Trashed {
		if err == nil {
			err = fmt.Errorf("โฟลเดอร์ต้นทางถูกลบหรือไม่ใช่โฟลเดอร์")
		}
		return 0, i.pauseRun(run, err)
	}
	if err := i.db.Model(run).Updates(map[string]any{"folder_name": folder.Name, "retry_at": nil, "last_error": ""}).Error; err != nil {
		return 0, err
	}
	if err := i.db.Model(&DriveSyncFolder{}).Where("run_id = ? AND folder_id = ?", run.ID, run.RootID).Update("drive_id", folder.DriveId).Error; err != nil {
		return 0, err
	}
	for ctx.Err() == nil {
		// Stop before another page when settings switch sources or disable auto
		// import. A manually requested batch may run with automation turned off.
		current, err := i.settings.LoadOCR()
		if err != nil {
			return imported, err
		}
		if current.ImportSource != "drive" || (!manual && !current.ImportEnabled) {
			return imported, nil
		}
		currentRoot, err := i.configuredFolderID(current.DriveFolderURL)
		if err != nil {
			return imported, err
		}
		if currentRoot != run.RootID {
			return imported, nil
		}
		var ownsLock bool
		if err := conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK('docflow:google-drive') = CONNECTION_ID()").Scan(&ownsLock); err != nil || !ownsLock {
			return imported, fmt.Errorf("Google Drive worker lost database lock")
		}
		worked := false
		var task DriveSyncFolder
		folderLookup := i.db.WithContext(ctx).Where("run_id = ? AND status = 'PENDING' AND (retry_at IS NULL OR retry_at <= ?)", run.ID, time.Now()).Order("folder_id").Limit(1).Find(&task)
		if folderLookup.Error != nil {
			return imported, folderLookup.Error
		}
		if folderLookup.RowsAffected > 0 {
			worked = true
			if err := i.discoverPage(ctx, run, &task); err != nil {
				if ctx.Err() != nil {
					return imported, nil
				}
				if err := i.failTask(&task, task.Attempts, err); err != nil {
					return imported, err
				}
			}
		}
		var items []DriveSyncItem
		if err := i.db.WithContext(ctx).Where("run_id = ? AND status = 'PENDING' AND (retry_at IS NULL OR retry_at <= ?)", run.ID, time.Now()).Order("file_id").Limit(25).Find(&items).Error; err != nil {
			return imported, err
		}
		for _, item := range items {
			if ctx.Err() != nil {
				return imported, nil
			}
			worked = true
			created, err := i.processItem(ctx, &item)
			if err != nil {
				if ctx.Err() != nil {
					return imported, nil
				}
				if err := i.failTask(&item, item.Attempts, err); err != nil {
					return imported, err
				}
			} else if created {
				imported++
			}
		}
		if !worked {
			var pendingFolders, pendingFiles int64
			if err := i.db.Model(&DriveSyncFolder{}).Where("run_id = ? AND status = 'PENDING'", run.ID).Count(&pendingFolders).Error; err != nil {
				return imported, err
			}
			if err := i.db.Model(&DriveSyncItem{}).Where("run_id = ? AND status = 'PENDING'", run.ID).Count(&pendingFiles).Error; err != nil {
				return imported, err
			}
			if pendingFolders+pendingFiles > 0 {
				return imported, nil
			}
			var failedFolders, failedFiles int64
			if err := i.db.Model(&DriveSyncFolder{}).Where("run_id = ? AND status = 'FAILED'", run.ID).Count(&failedFolders).Error; err != nil {
				return imported, err
			}
			if err := i.db.Model(&DriveSyncItem{}).Where("run_id = ? AND status = 'FAILED'", run.ID).Count(&failedFiles).Error; err != nil {
				return imported, err
			}
			status, message := "COMPLETED", ""
			if failedFolders+failedFiles > 0 {
				status = "PARTIAL"
				message = fmt.Sprintf("เข้าถึงโฟลเดอร์ไม่สำเร็จ %d รายการ และนำเข้าไฟล์ไม่สำเร็จ %d รายการ จะลองใหม่ในรอบถัดไป", failedFolders, failedFiles)
			}
			if err := i.db.Model(run).Updates(map[string]any{"status": status, "finished_at": time.Now(), "last_error": message}).Error; err != nil {
				return imported, err
			}
			details := fmt.Sprintf("สถานะ: %s; นำเข้าใหม่ %d รายการ", status, imported)
			if message != "" {
				details += "; " + message
			}
			i.audit("DRIVE_SYNC_FINISHED", details)
			return imported, nil
		}
	}
	return imported, nil
}

func (i *GoogleDriveImporter) pauseRun(run *DriveSyncRun, cause error) error {
	if err := i.db.Model(run).Updates(map[string]any{"last_error": cause.Error(), "retry_at": time.Now().Add(5 * time.Minute)}).Error; err != nil {
		return err
	}
	i.audit("DRIVE_SYNC_PAUSED", "รอรอบถัดไป: "+cause.Error())
	return cause
}

// audit records a Drive job event under the Google Drive job's audit identity.
// A failed audit write is logged; the sync itself carries on.
func (i *GoogleDriveImporter) audit(action, details string) {
	if err := i.db.Create(&AuditLog{UserID: auditSystemUser("google-drive-import"), Action: action, Details: details}).Error; err != nil {
		i.logger.Printf("Google Drive audit %s not written: %v", action, err)
	}
}

func (i *GoogleDriveImporter) failTask(task any, attempts int, cause error) error {
	status := "PENDING"
	if attempts >= 2 || !driveRetryable(cause) {
		status = "FAILED"
	}
	return i.db.Model(task).Updates(map[string]any{"status": status, "attempts": attempts + 1, "retry_at": time.Now().Add(time.Duration(attempts+1) * 5 * time.Minute), "last_error": cause.Error()}).Error
}

func (i *GoogleDriveImporter) getDriveFile(ctx context.Context, id, resourceKey string) (*drive.File, error) {
	return driveRetry(ctx, func() (*drive.File, error) {
		call := i.service.Files.Get(id).SupportsAllDrives(true).Fields(googleapi.Field(driveFileFields)).Context(ctx)
		driveResourceHeader(call.Header(), id, resourceKey)
		return call.Do()
	})
}

func (i *GoogleDriveImporter) listDriveFolderPage(ctx context.Context, folder *DriveSyncFolder) (*drive.FileList, error) {
	return driveRetry(ctx, func() (*drive.FileList, error) {
		call := i.service.Files.List().Q(fmt.Sprintf("'%s' in parents and trashed = false", escapeDriveQueryValue(folder.FolderID))).
			PageSize(drivePageSize).Fields(googleapi.Field("nextPageToken,incompleteSearch,files(" + driveFileFields + ")")).
			IncludeItemsFromAllDrives(true).SupportsAllDrives(true).Spaces("drive").PageToken(folder.PageToken).Context(ctx)
		if folder.DriveID != "" {
			call = call.Corpora("drive").DriveId(folder.DriveID)
		}
		driveResourceHeader(call.Header(), folder.FolderID, folder.ResourceKey)
		return call.Do()
	})
}

func (i *GoogleDriveImporter) discoverPage(ctx context.Context, run *DriveSyncRun, folder *DriveSyncFolder) error {
	page, err := i.listDriveFolderPage(ctx, folder)
	if err != nil {
		var apiError *googleapi.Error
		if folder.PageToken != "" && errors.As(err, &apiError) && apiError.Code == 400 {
			// Drive page tokens can be rejected after a long pause. Start this
			// folder again; unique task keys make replay safe.
			return i.db.Model(folder).Updates(map[string]any{"page_token": "", "attempts": 0, "retry_at": nil}).Error
		}
		return err
	}
	if page.IncompleteSearch {
		return fmt.Errorf("Google Drive ส่งผลค้นหาไม่ครบสำหรับโฟลเดอร์ %s", folder.FolderID)
	}
	return i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		folders := make([]DriveSyncFolder, 0)
		items := make([]DriveSyncItem, 0)
		for _, file := range page.Files {
			if !validDriveID(file.Id) {
				return fmt.Errorf("Drive returned an invalid file ID")
			}
			if file.MimeType == driveFolderMime {
				task := DriveSyncFolder{RunID: run.ID, FolderID: file.Id, ResourceKey: file.ResourceKey, DriveID: file.DriveId, Status: "PENDING"}
				folders = append(folders, task)
				continue
			}
			metadata, err := json.Marshal(file)
			if err != nil {
				return err
			}
			item := DriveSyncItem{RunID: run.ID, FileID: file.Id, FileName: truncateDriveName(file.Name), Metadata: string(metadata), Status: "PENDING"}
			items = append(items, item)
		}
		if len(folders) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&folders, 100).Error; err != nil {
				return err
			}
		}
		if len(items) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&items, 100).Error; err != nil {
				return err
			}
		}
		status := "PENDING"
		if page.NextPageToken == "" {
			status = "COMPLETED"
		}
		return tx.Model(folder).Updates(map[string]any{"page_token": page.NextPageToken, "status": status, "attempts": 0, "retry_at": nil, "last_error": ""}).Error
	})
}

func truncateDriveName(name string) string {
	runes := []rune(name)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return name
}

func (i *GoogleDriveImporter) processItem(ctx context.Context, item *DriveSyncItem) (bool, error) {
	var file drive.File
	if err := json.Unmarshal([]byte(item.Metadata), &file); err != nil {
		return false, err
	}
	if file.MimeType == driveShortcutMime {
		if file.ShortcutDetails == nil || !validDriveID(file.ShortcutDetails.TargetId) {
			return false, fmt.Errorf("ทางลัดไม่มีไฟล์ปลายทางที่เข้าถึงได้")
		}
		key := file.ShortcutDetails.TargetResourceKey
		target, err := i.getDriveFile(ctx, file.ShortcutDetails.TargetId, key)
		if err != nil {
			return false, err
		}
		if target.ResourceKey == "" {
			target.ResourceKey = key
		}
		if target.MimeType == driveShortcutMime {
			return false, fmt.Errorf("ปลายทางเป็นทางลัดซ้อนกัน")
		}
		file = *target
		if file.MimeType == driveFolderMime {
			err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				folder := DriveSyncFolder{RunID: item.RunID, FolderID: file.Id, ResourceKey: file.ResourceKey, DriveID: file.DriveId, Status: "PENDING"}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&folder).Error; err != nil {
					return err
				}
				return tx.Model(item).Updates(map[string]any{"status": "FOLDER", "last_error": ""}).Error
			})
			return false, err
		}
	}
	if file.Trashed {
		return false, i.finishItem(item, "SKIPPED", "ไฟล์อยู่ในถังขยะ")
	}
	// Refresh metadata for queued files: the file may have been renamed, moved,
	// replaced or deleted while a large initial import was in progress.
	fresh, err := i.getDriveFile(ctx, file.Id, file.ResourceKey)
	if err != nil {
		return false, err
	}
	if fresh.ResourceKey == "" {
		fresh.ResourceKey = file.ResourceKey
	}
	file = *fresh
	if file.Trashed {
		return false, i.finishItem(item, "SKIPPED", "ไฟล์อยู่ในถังขยะ")
	}
	var existing DriveImportedFile
	known := i.db.WithContext(ctx).Where("file_id = ?", file.Id).Limit(1).Find(&existing)
	if known.Error != nil {
		return false, known.Error
	}
	name, exportMime, supported := driveOutputFile(&file)
	if !supported {
		return false, i.finishItem(item, "SKIPPED", "ชนิดไฟล์ยังไม่รองรับ: "+file.MimeType)
	}
	isExisting := known.RowsAffected > 0
	if !isExisting && file.Size > maxImportedFileSize {
		return false, i.finishItem(item, "SKIPPED", "ไฟล์เกินขนาด 50 MB")
	}
	if !isExisting && file.Capabilities != nil && !file.Capabilities.CanDownload {
		return false, fmt.Errorf("บัญชีระบบไม่มีสิทธิ์ดาวน์โหลดไฟล์นี้")
	}
	file, name, webViewLink, err := i.renameDriveFile(ctx, &file, name)
	if err != nil {
		return false, err
	}
	item.FileName = truncateDriveName(name)
	if err := i.db.WithContext(ctx).Model(item).Update("file_name", item.FileName).Error; err != nil {
		return false, err
	}
	if isExisting {
		if err := i.updateImportedDriveSource(ctx, &existing, name, webViewLink); err != nil {
			return false, err
		}
		return false, i.finishItem(item, "EXISTING", "")
	}
	created, err := i.importFile(ctx, &file, name, exportMime, webViewLink, item)
	if err != nil {
		return false, err
	}
	return created, nil
}

func (i *GoogleDriveImporter) finishItem(item *DriveSyncItem, status, message string) error {
	return i.db.Model(item).Updates(map[string]any{"status": status, "last_error": message, "retry_at": nil}).Error
}

type DriveAutomationStatus struct {
	AccountEmail          string           `json:"accountEmail"`
	CredentialsConfigured bool             `json:"credentialsConfigured"`
	NextSyncAt            *time.Time       `json:"nextSyncAt"`
	Run                   *DriveSyncRun    `json:"run"`
	Counts                map[string]int64 `json:"counts"`
	FolderCounts          map[string]int64 `json:"folderCounts"`
	Issues                []DriveSyncItem  `json:"issues"`
	History               []DriveSyncRun   `json:"history"`
}

func googleDriveAccountEmail() string {
	return strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_ACCOUNT_EMAIL"))
}

func getDriveAutomationStatus(db *gorm.DB, store SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := store.LoadOCR()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "อ่านการตั้งค่า Drive ไม่สำเร็จ"})
			return
		}
		status := DriveAutomationStatus{AccountEmail: googleDriveAccountEmail(), CredentialsConfigured: googleDriveCredentialsConfigured(), Counts: map[string]int64{}, FolderCounts: map[string]int64{}, Issues: []DriveSyncItem{}, History: []DriveSyncRun{}}
		if settings.ImportEnabled && settings.ImportSource == "drive" {
			next := driveScheduledOccurrence(time.Now(), settings).AddDate(0, 0, 1)
			status.NextSyncAt = &next
		}
		root := normalizeDriveFolderID(settings.DriveFolderURL)
		if root == "" {
			root = normalizeDriveFolderID(getenv("GOOGLE_DRIVE_FOLDER_ID", ""))
		}
		if err := db.WithContext(c.Request.Context()).Where("root_id = ?", root).Order("id DESC").Limit(10).Find(&status.History).Error; err != nil {
			c.JSON(500, gin.H{"error": "อ่านประวัติการดึงไม่สำเร็จ"})
			return
		}
		if settings.ImportEnabled && settings.ImportSource == "drive" {
			now := time.Now()
			due := driveScheduledOccurrence(now, settings)
			newSourceDue := len(status.History) == 0 && due.Format("2006-01-02") == now.In(due.Location()).Format("2006-01-02")
			missed := len(status.History) > 0 && status.History[0].StartedAt.Before(due)
			if newSourceDue || missed {
				next := now.Truncate(time.Minute).Add(time.Minute)
				status.NextSyncAt = &next
			}
		}
		if len(status.History) > 0 {
			status.Run = &status.History[0]
			if status.Run.Status == "RUNNING" && (settings.ImportEnabled || status.Run.TriggerType == "MANUAL") && settings.ImportSource == "drive" {
				next := time.Now().Truncate(time.Minute).Add(time.Minute)
				if status.Run.RetryAt != nil && status.Run.RetryAt.After(next) {
					next = *status.Run.RetryAt
				}
				status.NextSyncAt = &next
			}
			for _, group := range []struct {
				model  any
				counts map[string]int64
			}{{&DriveSyncItem{}, status.Counts}, {&DriveSyncFolder{}, status.FolderCounts}} {
				var rows []struct {
					Status string
					Total  int64
				}
				if err := db.Model(group.model).Select("status, COUNT(*) AS total").Where("run_id = ?", status.Run.ID).Group("status").Scan(&rows).Error; err != nil {
					c.JSON(500, gin.H{"error": "อ่านความคืบหน้าไม่สำเร็จ"})
					return
				}
				for _, row := range rows {
					group.counts[row.Status] = row.Total
				}
			}
			if err := db.Where("run_id = ? AND (status IN ('FAILED', 'SKIPPED') OR last_error <> '')", status.Run.ID).Order("file_id").Limit(20).Find(&status.Issues).Error; err != nil {
				c.JSON(500, gin.H{"error": "อ่านรายการปัญหาไม่สำเร็จ"})
				return
			}
			var folderIssues []DriveSyncFolder
			if err := db.Where("run_id = ? AND last_error <> ''", status.Run.ID).Limit(20).Find(&folderIssues).Error; err != nil {
				c.JSON(500, gin.H{"error": "อ่านปัญหาโฟลเดอร์ไม่สำเร็จ"})
				return
			}
			for _, issue := range folderIssues {
				status.Issues = append(status.Issues, DriveSyncItem{FileName: "โฟลเดอร์ " + issue.FolderID, Status: issue.Status, LastError: issue.LastError})
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": status})
	}
}

func validateDriveCredentials(contents []byte) error {
	var credentials struct {
		Type         string `json:"type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(contents, &credentials); err != nil {
		return fmt.Errorf("อ่าน Google OAuth credential JSON ไม่สำเร็จ")
	}
	if credentials.Type != "authorized_user" || credentials.ClientID == "" || credentials.ClientSecret == "" || credentials.RefreshToken == "" {
		return fmt.Errorf("Google Drive ต้องใช้ OAuth credential ของบัญชี Google (authorized_user) พร้อม refresh token")
	}
	return nil
}

package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"gorm.io/gorm"
)

// Imports survive restarts. A database lock limits Drive OCR to one document
// at a time across backend instances and allows interrupted jobs to resume.
func runDriveOCRQueue(db *gorm.DB, processor *DocumentProcessor, logger *log.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := db.DB()
	if err != nil {
		logger.Printf("Drive OCR queue: %v", err)
		return
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		logger.Printf("Drive OCR queue: %v", err)
		return
	}
	defer conn.Close()
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('docflow:drive-ocr', 0)").Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		return
	}
	defer func() {
		releaseCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = conn.ExecContext(releaseCtx, "SELECT RELEASE_LOCK('docflow:drive-ocr')")
	}()
	for ctx.Err() == nil {
		var ownsLock bool
		if err := conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK('docflow:drive-ocr') = CONNECTION_ID()").Scan(&ownsLock); err != nil || !ownsLock {
			return
		}
		var document Document
		lookup := db.WithContext(ctx).Where("status <> ?", documentCancelled).Where("import_source = ? AND processing_stage IN ?", "GOOGLE_DRIVE", []string{"IMPORTED", "QUEUED", "PREPARING"}).Order("id").Limit(1).Find(&document)
		if lookup.Error != nil {
			logger.Printf("Drive OCR queue: %v", lookup.Error)
			return
		}
		if lookup.RowsAffected == 0 {
			return
		}
		// Give each document its own deadline so the end of a queue batch does
		// not fail a newly started OCR job.
		documentCtx, stop := context.WithTimeout(context.Background(), 10*time.Minute)
		processErr := processor.Process(documentCtx, document.ID)
		stop()
		if processErr != nil {
			logger.Printf("Drive OCR document %d: %v", document.ID, processErr)
		}
	}
}

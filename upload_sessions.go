package main

import (
	"database/sql"
	"strings"
	"time"
)

const (
	uploadSessionStatusPending   = "pending"
	uploadSessionStatusUploading = "uploading"
	uploadSessionStatusCompleted = "completed"
	uploadSessionStatusAborted   = "aborted"
	uploadSessionStatusFailed    = "failed"
)

type uploadSession struct {
	SessionID         string
	Mode              string
	ShareCode         string
	FileName          string
	FileSize          int64
	FileHash          string
	ObjectKey         string
	MultipartUploadID string
	PartSize          int64
	TotalParts        int
	Status            string
	ExpireAt          int64
}

func ensureUploadSessionTable() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS upload_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL UNIQUE,
		mode TEXT NOT NULL,
		share_code TEXT NOT NULL UNIQUE,
		file_name TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		file_hash TEXT NOT NULL,
		object_key TEXT NOT NULL,
		multipart_upload_id TEXT NOT NULL,
		part_size INTEGER NOT NULL,
		total_parts INTEGER NOT NULL,
		status TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		expire_at INTEGER NOT NULL
	);`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_upload_sessions_status_updated
		ON upload_sessions (status, updated_at);`); err != nil {
		return err
	}
	return nil
}

func insertUploadSession(session uploadSession) error {
	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO upload_sessions
		(session_id, mode, share_code, file_name, file_size, file_hash, object_key, multipart_upload_id, part_size, total_parts, status, created_at, updated_at, expire_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(session.SessionID),
		strings.TrimSpace(session.Mode),
		strings.TrimSpace(session.ShareCode),
		strings.TrimSpace(session.FileName),
		session.FileSize,
		strings.TrimSpace(session.FileHash),
		strings.TrimSpace(session.ObjectKey),
		strings.TrimSpace(session.MultipartUploadID),
		session.PartSize,
		session.TotalParts,
		strings.TrimSpace(session.Status),
		now,
		now,
		session.ExpireAt,
	)
	return err
}

func loadUploadSession(sessionID string) (uploadSession, error) {
	var session uploadSession
	err := db.QueryRow(`SELECT session_id, mode, share_code, file_name, file_size, file_hash, object_key, multipart_upload_id, part_size, total_parts, status, expire_at
		FROM upload_sessions WHERE session_id = ?`,
		strings.TrimSpace(sessionID),
	).Scan(
		&session.SessionID,
		&session.Mode,
		&session.ShareCode,
		&session.FileName,
		&session.FileSize,
		&session.FileHash,
		&session.ObjectKey,
		&session.MultipartUploadID,
		&session.PartSize,
		&session.TotalParts,
		&session.Status,
		&session.ExpireAt,
	)
	return session, err
}

func updateUploadSessionStatus(sessionID, status string) error {
	_, err := db.Exec(`UPDATE upload_sessions SET status = ?, updated_at = ? WHERE session_id = ?`,
		strings.TrimSpace(status),
		time.Now().Unix(),
		strings.TrimSpace(sessionID),
	)
	return err
}

func deleteUploadSession(sessionID string) error {
	_, err := db.Exec(`DELETE FROM upload_sessions WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return err
}

func listStaleUploadSessions(staleBefore int64, limit int) ([]uploadSession, error) {
	rows, err := db.Query(`SELECT session_id, mode, share_code, file_name, file_size, file_hash, object_key, multipart_upload_id, part_size, total_parts, status, expire_at
		FROM upload_sessions
		WHERE status IN (?, ?) AND updated_at < ?
		ORDER BY updated_at ASC
		LIMIT ?`,
		uploadSessionStatusPending,
		uploadSessionStatusUploading,
		staleBefore,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []uploadSession
	for rows.Next() {
		var session uploadSession
		if err := rows.Scan(
			&session.SessionID,
			&session.Mode,
			&session.ShareCode,
			&session.FileName,
			&session.FileSize,
			&session.FileHash,
			&session.ObjectKey,
			&session.MultipartUploadID,
			&session.PartSize,
			&session.TotalParts,
			&session.Status,
			&session.ExpireAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func sumActiveUploadSessionBytes(excludeSessionID string) (int64, error) {
	var size sql.NullInt64
	err := db.QueryRow(`SELECT COALESCE(SUM(file_size), 0)
		FROM upload_sessions
		WHERE status IN (?, ?)
		  AND (? = '' OR session_id <> ?)`,
		uploadSessionStatusPending,
		uploadSessionStatusUploading,
		strings.TrimSpace(excludeSessionID),
		strings.TrimSpace(excludeSessionID),
	).Scan(&size)
	if err != nil {
		return 0, err
	}
	return size.Int64, nil
}

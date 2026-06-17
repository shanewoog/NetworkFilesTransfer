package main

import (
	"database/sql"
	"log"
	"strings"
	"time"
)

const (
	replicaBackendR2       = "r2"
	replicaStatusPending   = "pending"
	replicaStatusUploading = "uploading"
	replicaStatusUploaded  = "uploaded"
	replicaStatusFailed    = "failed"
)

type fileReplicaJob struct {
	FileCode  string
	Backend   string
	ObjectKey string
	Status    string
	LocalPath string
}

func ensureFileReplicaTable() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_replicas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_code TEXT NOT NULL,
		backend TEXT NOT NULL,
		object_key TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		uploaded_at INTEGER DEFAULT 0,
		UNIQUE(file_code, backend)
	);`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_file_replicas_backend_status
		ON file_replicas (backend, status, updated_at);`); err != nil {
		return err
	}
	return ensureFileReplicaUploadTables()
}

func upsertFileReplicaPending(fileCode, backend, objectKey string) error {
	fileCode = strings.TrimSpace(fileCode)
	backend = strings.TrimSpace(backend)
	objectKey = strings.TrimSpace(objectKey)
	if fileCode == "" || backend == "" || objectKey == "" {
		return nil
	}

	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO file_replicas
		(file_code, backend, object_key, status, error, created_at, updated_at, uploaded_at)
		VALUES (?, ?, ?, ?, '', ?, ?, 0)
		ON CONFLICT(file_code, backend) DO UPDATE SET
			object_key = excluded.object_key,
			status = excluded.status,
			error = '',
			updated_at = excluded.updated_at,
			uploaded_at = 0`,
		fileCode, backend, objectKey, replicaStatusPending, now, now,
	)
	return err
}

func markFileReplicaUploading(fileCode, backend string) error {
	return updateFileReplicaStatus(fileCode, backend, replicaStatusUploading, "", 0)
}

func markFileReplicaUploaded(fileCode, backend string) error {
	return updateFileReplicaStatus(fileCode, backend, replicaStatusUploaded, "", time.Now().Unix())
}

func markFileReplicaFailed(fileCode, backend, errText string) error {
	return updateFileReplicaStatus(fileCode, backend, replicaStatusFailed, errText, 0)
}

func updateFileReplicaStatus(fileCode, backend, status, errText string, uploadedAt int64) error {
	fileCode = strings.TrimSpace(fileCode)
	backend = strings.TrimSpace(backend)
	if fileCode == "" || backend == "" {
		return nil
	}

	now := time.Now().Unix()
	_, err := db.Exec(`UPDATE file_replicas
		SET status = ?, error = ?, updated_at = ?, uploaded_at = ?
		WHERE file_code = ? AND backend = ?`,
		status, strings.TrimSpace(errText), now, uploadedAt, fileCode, backend,
	)
	return err
}

func loadFileReplicaJob(fileCode, backend string) (fileReplicaJob, error) {
	var job fileReplicaJob
	err := db.QueryRow(`SELECT fr.file_code, fr.backend, fr.object_key, fr.status, f.path
		FROM file_replicas fr
		JOIN files f ON f.code = fr.file_code
		WHERE fr.file_code = ? AND fr.backend = ?`,
		fileCode, backend,
	).Scan(&job.FileCode, &job.Backend, &job.ObjectKey, &job.Status, &job.LocalPath)
	return job, err
}

func getUploadedReplicaObjectKey(fileCode, backend string) (string, error) {
	var objectKey string
	err := db.QueryRow(`SELECT object_key
		FROM file_replicas
		WHERE file_code = ? AND backend = ? AND status = ?`,
		strings.TrimSpace(fileCode), strings.TrimSpace(backend), replicaStatusUploaded,
	).Scan(&objectKey)
	return objectKey, err
}

func hasUploadedReplica(fileCode, backend string) (bool, error) {
	var exists int
	err := db.QueryRow(`SELECT 1
		FROM file_replicas
		WHERE file_code = ? AND backend = ? AND status = ?
		LIMIT 1`,
		strings.TrimSpace(fileCode), strings.TrimSpace(backend), replicaStatusUploaded,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func listReplicaCodesNeedingSync(backend string, limit int) ([]string, error) {
	rows, err := db.Query(`SELECT file_code
		FROM file_replicas
		WHERE backend = ? AND status IN (?, ?, ?)
		ORDER BY updated_at ASC
		LIMIT ?`,
		backend, replicaStatusPending, replicaStatusFailed, replicaStatusUploading, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

func deleteFileReplicaRecordsByCode(code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	if err := deleteFileReplicaMultipartState(code, ""); err != nil && err != sql.ErrNoRows {
		log.Printf("delete replica multipart state failed: code=%s, err=%v", code, err)
	}
	if _, err := db.Exec("DELETE FROM file_replicas WHERE file_code = ?", code); err != nil && err != sql.ErrNoRows {
		log.Printf("delete replica records failed: code=%s, err=%v", code, err)
	}
}

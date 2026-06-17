package main

import (
	"database/sql"
	"strings"
	"time"
)

type fileReplicaUploadSession struct {
	FileCode   string
	Backend    string
	ObjectKey  string
	UploadID   string
	PartSize   int64
	TotalParts int
	Status     string
}

type fileReplicaUploadedPart struct {
	PartNumber int32
	ETag       string
	PartSize   int64
}

func ensureFileReplicaUploadTables() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_replica_uploads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_code TEXT NOT NULL,
		backend TEXT NOT NULL,
		object_key TEXT NOT NULL,
		upload_id TEXT NOT NULL,
		part_size INTEGER NOT NULL,
		total_parts INTEGER NOT NULL,
		status TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(file_code, backend)
	);`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_file_replica_uploads_backend_status
		ON file_replica_uploads (backend, status, updated_at);`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_replica_parts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_code TEXT NOT NULL,
		backend TEXT NOT NULL,
		upload_id TEXT NOT NULL,
		part_number INTEGER NOT NULL,
		etag TEXT NOT NULL,
		part_size INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(file_code, backend, upload_id, part_number)
	);`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_file_replica_parts_upload
		ON file_replica_parts (file_code, backend, upload_id, part_number);`); err != nil {
		return err
	}
	return nil
}

func upsertFileReplicaUploadSession(session fileReplicaUploadSession) error {
	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO file_replica_uploads
		(file_code, backend, object_key, upload_id, part_size, total_parts, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_code, backend) DO UPDATE SET
			object_key = excluded.object_key,
			upload_id = excluded.upload_id,
			part_size = excluded.part_size,
			total_parts = excluded.total_parts,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(session.FileCode),
		strings.TrimSpace(session.Backend),
		strings.TrimSpace(session.ObjectKey),
		strings.TrimSpace(session.UploadID),
		session.PartSize,
		session.TotalParts,
		strings.TrimSpace(session.Status),
		now,
		now,
	)
	return err
}

func updateFileReplicaUploadSessionStatus(fileCode, backend, status string) error {
	_, err := db.Exec(`UPDATE file_replica_uploads
		SET status = ?, updated_at = ?
		WHERE file_code = ? AND backend = ?`,
		strings.TrimSpace(status),
		time.Now().Unix(),
		strings.TrimSpace(fileCode),
		strings.TrimSpace(backend),
	)
	return err
}

func loadFileReplicaUploadSession(fileCode, backend string) (fileReplicaUploadSession, error) {
	var session fileReplicaUploadSession
	err := db.QueryRow(`SELECT file_code, backend, object_key, upload_id, part_size, total_parts, status
		FROM file_replica_uploads
		WHERE file_code = ? AND backend = ?`,
		strings.TrimSpace(fileCode),
		strings.TrimSpace(backend),
	).Scan(
		&session.FileCode,
		&session.Backend,
		&session.ObjectKey,
		&session.UploadID,
		&session.PartSize,
		&session.TotalParts,
		&session.Status,
	)
	return session, err
}

func saveFileReplicaUploadedPart(fileCode, backend, uploadID string, partNumber int32, etag string, partSize int64) error {
	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO file_replica_parts
		(file_code, backend, upload_id, part_number, etag, part_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_code, backend, upload_id, part_number) DO UPDATE SET
			etag = excluded.etag,
			part_size = excluded.part_size,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(fileCode),
		strings.TrimSpace(backend),
		strings.TrimSpace(uploadID),
		partNumber,
		strings.TrimSpace(etag),
		partSize,
		now,
		now,
	)
	return err
}

func listFileReplicaUploadedParts(fileCode, backend, uploadID string) ([]fileReplicaUploadedPart, error) {
	rows, err := db.Query(`SELECT part_number, etag, part_size
		FROM file_replica_parts
		WHERE file_code = ? AND backend = ? AND upload_id = ?
		ORDER BY part_number ASC`,
		strings.TrimSpace(fileCode),
		strings.TrimSpace(backend),
		strings.TrimSpace(uploadID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parts []fileReplicaUploadedPart
	for rows.Next() {
		var part fileReplicaUploadedPart
		if err := rows.Scan(&part.PartNumber, &part.ETag, &part.PartSize); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

func deleteFileReplicaMultipartState(fileCode, backend string) error {
	fileCode = strings.TrimSpace(fileCode)
	backend = strings.TrimSpace(backend)
	if fileCode == "" {
		return nil
	}

	if backend == "" {
		if _, err := db.Exec("DELETE FROM file_replica_parts WHERE file_code = ?", fileCode); err != nil && err != sql.ErrNoRows {
			return err
		}
		if _, err := db.Exec("DELETE FROM file_replica_uploads WHERE file_code = ?", fileCode); err != nil && err != sql.ErrNoRows {
			return err
		}
		return nil
	}

	if _, err := db.Exec("DELETE FROM file_replica_parts WHERE file_code = ? AND backend = ?", fileCode, backend); err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := db.Exec("DELETE FROM file_replica_uploads WHERE file_code = ? AND backend = ?", fileCode, backend); err != nil && err != sql.ErrNoRows {
		return err
	}
	return nil
}

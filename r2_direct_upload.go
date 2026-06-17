package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	directUploadURLExpiry  = 15 * time.Minute
	staleUploadSessionAge  = 6 * time.Hour
	staleUploadSessionScan = 256
)

type R2DirectUploader struct {
	cfg     R2Config
	client  *s3.Client
	presign *s3.PresignClient
}

var r2DirectUploader *R2DirectUploader

func initR2DirectUploader() {
	uploader, err := newR2DirectUploader(globalR2Cfg)
	if err != nil {
		log.Printf("init direct R2 uploader failed: %v", err)
		return
	}
	r2DirectUploader = uploader
}

func newR2DirectUploader(cfg R2Config) (*R2DirectUploader, error) {
	cfg = cfg.normalized()
	if !cfg.isEnabled() {
		return nil, nil
	}
	if !cfg.isComplete() {
		return nil, fmt.Errorf("R2 config is incomplete: endpoint/bucket/access_key_id/secret_access_key are required")
	}

	awsCfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		// R2 browser direct upload works more reliably with bucket-in-host
		// presigned URLs, e.g. https://<bucket>.<account>.r2.cloudflarestorage.com/...
		o.UsePathStyle = false
	})

	return &R2DirectUploader{
		cfg:     cfg,
		client:  client,
		presign: s3.NewPresignClient(client),
	}, nil
}

func directUploadGuard(c *gin.Context) bool {
	if !isR2DirectUploadMode() {
		c.JSON(http.StatusNotFound, gin.H{"error": "r2 direct upload mode is disabled"})
		return false
	}
	if r2DirectUploader == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "r2 direct uploader is unavailable"})
		return false
	}
	return true
}

func handleR2UploadInit(c *gin.Context) {
	if !directUploadGuard(c) {
		return
	}

	var req struct {
		FileName string `json:"file_name"`
		FileSize int64  `json:"file_size"`
		Hash     string `json:"hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	req.FileName = sanitizeFileName(req.FileName)
	req.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
	if req.FileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty file name"})
		return
	}
	if req.FileSize <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty file is not supported"})
		return
	}
	if !isValidFileHash(req.Hash) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file hash"})
		return
	}
	if req.FileSize > globalCfg.MaxSingleSizeGB*1024*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file exceeds size limit"})
		return
	}
	if ok, err := canReserveManagedStorage(req.FileSize, ""); err != nil {
		log.Printf("check managed storage for direct upload init failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage check failed"})
		return
	} else if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage limit reached"})
		return
	}

	partSize, totalParts := buildR2MultipartPlan(req.FileSize)
	if totalParts <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid multipart plan"})
		return
	}

	shareCode, err := reserveDirectUploadShareCode()
	if err != nil {
		log.Printf("reserve share code for direct upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create upload session failed"})
		return
	}

	sessionID := uuid.NewString()
	objectKey := buildR2ObjectKey(r2DirectUploader.cfg.Prefix, shareCode, req.FileName, "")

	createOut, err := r2DirectUploader.client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(r2DirectUploader.cfg.Bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		log.Printf("create direct R2 multipart upload failed: code=%s, key=%s, err=%v", shareCode, objectKey, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create upload session failed"})
		return
	}

	session := uploadSession{
		SessionID:         sessionID,
		Mode:              uploadModeR2Direct,
		ShareCode:         shareCode,
		FileName:          req.FileName,
		FileSize:          req.FileSize,
		FileHash:          req.Hash,
		ObjectKey:         objectKey,
		MultipartUploadID: strings.TrimSpace(aws.ToString(createOut.UploadId)),
		PartSize:          partSize,
		TotalParts:        totalParts,
		Status:            uploadSessionStatusUploading,
		ExpireAt:          time.Now().Add(time.Duration(globalCfg.ExpireHours) * time.Hour).Unix(),
	}
	if session.MultipartUploadID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create upload session failed"})
		return
	}

	if err := upsertFileReplicaPending(session.ShareCode, replicaBackendR2, session.ObjectKey); err != nil {
		_, _ = r2DirectUploader.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(r2DirectUploader.cfg.Bucket),
			Key:      aws.String(session.ObjectKey),
			UploadId: aws.String(session.MultipartUploadID),
		})
		log.Printf("create replica record for direct upload failed: session=%s, err=%v", sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create upload session failed"})
		return
	}

	if err := insertUploadSession(session); err != nil {
		_, _ = r2DirectUploader.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(r2DirectUploader.cfg.Bucket),
			Key:      aws.String(session.ObjectKey),
			UploadId: aws.String(session.MultipartUploadID),
		})
		deleteFileReplicaRecordsByCode(session.ShareCode)
		log.Printf("save direct upload session failed: session=%s, err=%v", sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create upload session failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":  session.SessionID,
		"chunk_size":  session.PartSize,
		"parallel":    directUploadParallel(session.FileSize),
		"total_parts": session.TotalParts,
	})
}

func handleR2UploadSignPart(c *gin.Context) {
	if !directUploadGuard(c) {
		return
	}

	var req struct {
		SessionID  string `json:"session_id"`
		PartNumber int    `json:"part_number"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	session, err := loadUploadSession(req.SessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "upload session not found"})
			return
		}
		log.Printf("load direct upload session failed: session=%s, err=%v", req.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load upload session failed"})
		return
	}

	if session.Mode != uploadModeR2Direct {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload session"})
		return
	}
	if session.Status != uploadSessionStatusUploading && session.Status != uploadSessionStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload session is not active"})
		return
	}
	if req.PartNumber < 1 || req.PartNumber > session.TotalParts {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid part number"})
		return
	}

	if err := updateUploadSessionStatus(session.SessionID, uploadSessionStatusUploading); err != nil {
		log.Printf("touch direct upload session failed: session=%s, err=%v", session.SessionID, err)
	}

	presigned, err := r2DirectUploader.presign.PresignUploadPart(
		context.Background(),
		&s3.UploadPartInput{
			Bucket:     aws.String(r2DirectUploader.cfg.Bucket),
			Key:        aws.String(session.ObjectKey),
			UploadId:   aws.String(session.MultipartUploadID),
			PartNumber: aws.Int32(int32(req.PartNumber)),
		},
		func(options *s3.PresignOptions) {
			options.Expires = directUploadURLExpiry
		},
	)
	if err != nil {
		log.Printf("presign direct R2 upload part failed: session=%s, part=%d, err=%v", session.SessionID, req.PartNumber, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign upload part failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":         presigned.URL,
		"part_number": req.PartNumber,
	})
}

func handleR2UploadComplete(c *gin.Context) {
	if !directUploadGuard(c) {
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Parts     []struct {
			PartNumber int    `json:"part_number"`
			ETag       string `json:"etag"`
		} `json:"parts"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	session, err := loadUploadSession(req.SessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "upload session not found"})
			return
		}
		log.Printf("load direct upload session for completion failed: session=%s, err=%v", req.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load upload session failed"})
		return
	}

	completedParts, err := buildCompletedParts(req.Parts, session.TotalParts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := r2DirectUploader.client.CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(r2DirectUploader.cfg.Bucket),
		Key:      aws.String(session.ObjectKey),
		UploadId: aws.String(session.MultipartUploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}); err != nil {
		log.Printf("complete direct R2 multipart upload failed: session=%s, err=%v", session.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "complete upload failed"})
		return
	}

	if _, err := db.Exec(`INSERT INTO files (code, name, path, size, expire_at, hash, download_count, download_limit, primary_backend)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ShareCode,
		session.FileName,
		"",
		session.FileSize,
		session.ExpireAt,
		session.FileHash,
		0,
		globalCfg.DownloadLimit,
		replicaBackendR2,
	); err != nil {
		log.Printf("insert file record after direct R2 upload failed: session=%s, code=%s, err=%v", session.SessionID, session.ShareCode, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save file record failed"})
		return
	}

	if err := markFileReplicaUploaded(session.ShareCode, replicaBackendR2); err != nil {
		log.Printf("mark direct R2 upload replica as uploaded failed: session=%s, code=%s, err=%v", session.SessionID, session.ShareCode, err)
	}
	if err := updateUploadSessionStatus(session.SessionID, uploadSessionStatusCompleted); err != nil {
		log.Printf("mark direct upload session completed failed: session=%s, err=%v", session.SessionID, err)
	}
	if err := deleteUploadSession(session.SessionID); err != nil {
		log.Printf("delete completed direct upload session failed: session=%s, err=%v", session.SessionID, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      session.ShareCode,
		"url":       "/api/download/" + session.ShareCode,
		"expire_at": session.ExpireAt,
	})
}

func handleR2UploadCancel(c *gin.Context) {
	if !directUploadGuard(c) {
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := abortDirectUploadSession(strings.TrimSpace(req.SessionID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		log.Printf("cancel direct R2 upload failed: session=%s, err=%v", req.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel upload failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func abortDirectUploadSession(sessionID string) error {
	session, err := loadUploadSession(sessionID)
	if err != nil {
		return err
	}

	if r2DirectUploader != nil && session.MultipartUploadID != "" && session.ObjectKey != "" {
		_, abortErr := r2DirectUploader.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(r2DirectUploader.cfg.Bucket),
			Key:      aws.String(session.ObjectKey),
			UploadId: aws.String(session.MultipartUploadID),
		})
		if abortErr != nil && !isNoSuchUploadError(abortErr) {
			return abortErr
		}
	}

	if err := updateUploadSessionStatus(session.SessionID, uploadSessionStatusAborted); err != nil {
		return err
	}
	if err := deleteUploadSession(session.SessionID); err != nil {
		return err
	}
	deleteFileReplicaRecordsByCode(session.ShareCode)
	return nil
}

func cleanupStaleUploadSessionsOnce() {
	if r2DirectUploader == nil {
		return
	}

	staleBefore := time.Now().Add(-staleUploadSessionAge).Unix()
	sessions, err := listStaleUploadSessions(staleBefore, staleUploadSessionScan)
	if err != nil {
		log.Printf("list stale direct upload sessions failed: %v", err)
		return
	}

	for _, session := range sessions {
		if err := abortDirectUploadSession(session.SessionID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("cleanup stale direct upload session failed: session=%s, err=%v", session.SessionID, err)
		}
	}
}

func directUploadParallel(fileSize int64) int {
	switch {
	case fileSize < 200*1024*1024:
		return 1
	case fileSize < 1024*1024*1024:
		return 2
	default:
		return 3
	}
}

func reserveDirectUploadShareCode() (string, error) {
	for i := 0; i < 16; i++ {
		code := randomString(globalCfg.ShareCodeLength)
		var exists int
		err := db.QueryRow(`SELECT 1 FROM files WHERE code = ? LIMIT 1`, code).Scan(&exists)
		if err == sql.ErrNoRows {
			err = db.QueryRow(`SELECT 1 FROM upload_sessions WHERE share_code = ? LIMIT 1`, code).Scan(&exists)
			if err == sql.ErrNoRows {
				return code, nil
			}
		}
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to reserve unique share code")
}

func buildCompletedParts(parts []struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}, expectedTotal int) ([]s3types.CompletedPart, error) {
	if expectedTotal <= 0 {
		return nil, fmt.Errorf("invalid expected part count")
	}
	if len(parts) != expectedTotal {
		return nil, fmt.Errorf("incomplete parts")
	}

	partMap := make(map[int]string, len(parts))
	for _, part := range parts {
		etag := strings.TrimSpace(part.ETag)
		if part.PartNumber < 1 || part.PartNumber > expectedTotal || etag == "" {
			return nil, fmt.Errorf("invalid parts")
		}
		partMap[part.PartNumber] = etag
	}
	if len(partMap) != expectedTotal {
		return nil, fmt.Errorf("duplicate parts")
	}

	partNumbers := make([]int, 0, len(partMap))
	for partNumber := range partMap {
		partNumbers = append(partNumbers, partNumber)
	}
	sort.Ints(partNumbers)

	completedParts := make([]s3types.CompletedPart, 0, len(partNumbers))
	for _, partNumber := range partNumbers {
		etag := partMap[partNumber]
		completedParts = append(completedParts, s3types.CompletedPart{
			ETag:       aws.String(etag),
			PartNumber: aws.Int32(int32(partNumber)),
		})
	}
	return completedParts, nil
}

func currentManagedStorageBytes(excludeSessionID string) (int64, error) {
	now := time.Now().Unix()

	var fileBytes sql.NullInt64
	if err := db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM files WHERE expire_at > ?`, now).Scan(&fileBytes); err != nil {
		return 0, err
	}

	sessionBytes, err := sumActiveUploadSessionBytes(excludeSessionID)
	if err != nil {
		return 0, err
	}

	return fileBytes.Int64 + sessionBytes, nil
}

func canReserveManagedStorage(extraBytes int64, excludeSessionID string) (bool, error) {
	usedBytes, err := currentManagedStorageBytes(excludeSessionID)
	if err != nil {
		return false, err
	}
	limitBytes := globalCfg.MaxTotalSizeGB * 1024 * 1024 * 1024
	return usedBytes+extraBytes <= limitBytes, nil
}

func parsePartNumber(raw string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(raw))
}

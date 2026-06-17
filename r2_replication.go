package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	r2ReplicaQueueSize       = 128
	r2ReplicaRecoverLimit    = 256
	r2ReplicaRecoverInterval = time.Minute
)

var (
	r2Replicator         *R2Replicator
	r2ReplicationInitErr error
)

type R2Replicator struct {
	cfg    R2Config
	client *s3.Client
	queue  chan string

	mu     sync.Mutex
	queued map[string]struct{}
}

func initR2Replication() {
	if effectiveUploadMode() != uploadModeLocalThenSync {
		return
	}

	replicator, err := newR2Replicator(globalR2Cfg)
	if err != nil {
		r2ReplicationInitErr = err
		log.Printf("init R2 replication failed: %v", err)
		return
	}
	r2Replicator = replicator
	if r2Replicator == nil {
		return
	}
	r2Replicator.Start()
}

func newR2Replicator(cfg R2Config) (*R2Replicator, error) {
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
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})

	return &R2Replicator{
		cfg:    cfg,
		client: client,
		queue:  make(chan string, r2ReplicaQueueSize),
		queued: make(map[string]struct{}),
	}, nil
}

func (r *R2Replicator) Start() {
	go r.run()
	go r.recoverLoop()
	r.enqueuePendingReplicas()
}

func scheduleR2ReplicaUpload(fileCode, localPath, originalName string) error {
	if !globalR2Cfg.isEnabled() {
		return nil
	}
	if r2ReplicationInitErr != nil {
		return r2ReplicationInitErr
	}
	if r2Replicator == nil {
		return fmt.Errorf("R2 replicator is not initialized")
	}

	objectKey := buildR2ObjectKey(r2Replicator.cfg.Prefix, fileCode, originalName, localPath)
	if err := upsertFileReplicaPending(fileCode, replicaBackendR2, objectKey); err != nil {
		return err
	}
	r2Replicator.Enqueue(fileCode)
	return nil
}

func buildR2ObjectKey(prefix, fileCode, originalName, localPath string) string {
	objectName := sanitizeFileName(strings.TrimSpace(originalName))
	if objectName == "" || objectName == "file" {
		objectName = filepath.Base(strings.TrimSpace(localPath))
	}

	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	fileCode = strings.TrimSpace(fileCode)

	switch {
	case prefix == "" && fileCode == "":
		return objectName
	case prefix == "":
		return fileCode + "/" + objectName
	case fileCode == "":
		return prefix + "/" + objectName
	default:
		return prefix + "/" + fileCode + "/" + objectName
	}
}

func (r *R2Replicator) Enqueue(fileCode string) {
	fileCode = strings.TrimSpace(fileCode)
	if fileCode == "" {
		return
	}

	r.mu.Lock()
	if _, exists := r.queued[fileCode]; exists {
		r.mu.Unlock()
		return
	}
	r.queued[fileCode] = struct{}{}
	r.mu.Unlock()

	select {
	case r.queue <- fileCode:
	default:
		r.unmarkQueued(fileCode)
		log.Printf("R2 queue is full, skip enqueue: code=%s", fileCode)
	}
}

func (r *R2Replicator) run() {
	for fileCode := range r.queue {
		r.process(fileCode)
		r.unmarkQueued(fileCode)
	}
}

func (r *R2Replicator) recoverLoop() {
	ticker := time.NewTicker(r2ReplicaRecoverInterval)
	defer ticker.Stop()

	for range ticker.C {
		r.enqueuePendingReplicas()
	}
}

func (r *R2Replicator) enqueuePendingReplicas() {
	codes, err := listReplicaCodesNeedingSync(replicaBackendR2, r2ReplicaRecoverLimit)
	if err != nil {
		log.Printf("list pending R2 replicas failed: %v", err)
		return
	}
	for _, code := range codes {
		r.Enqueue(code)
	}
}

func (r *R2Replicator) process(fileCode string) {
	job, err := loadFileReplicaJob(fileCode, replicaBackendR2)
	if err != nil {
		if err == sql.ErrNoRows {
			return
		}
		log.Printf("load R2 replica job failed: code=%s, err=%v", fileCode, err)
		return
	}

	if strings.TrimSpace(job.LocalPath) == "" {
		if err := r.abortAndDeleteMultipartState(fileCode, replicaBackendR2); err != nil {
			log.Printf("abort R2 multipart state for empty local path failed: code=%s, err=%v", fileCode, err)
		}
		deleteFileReplicaRecordsByCode(fileCode)
		return
	}

	if _, err := os.Stat(job.LocalPath); err != nil {
		if os.IsNotExist(err) {
			if err := r.abortAndDeleteMultipartState(fileCode, replicaBackendR2); err != nil {
				log.Printf("abort R2 multipart state for missing local file failed: code=%s, err=%v", fileCode, err)
			}
			deleteFileReplicaRecordsByCode(fileCode)
			return
		}
		_ = markFileReplicaFailed(fileCode, replicaBackendR2, err.Error())
		log.Printf("stat local file for R2 upload failed: code=%s, path=%s, err=%v", fileCode, job.LocalPath, err)
		return
	}

	if err := markFileReplicaUploading(fileCode, replicaBackendR2); err != nil {
		log.Printf("mark R2 replica as uploading failed: code=%s, err=%v", fileCode, err)
		return
	}

	if err := r.uploadLocalFileMultipart(fileCode, job.ObjectKey, job.LocalPath); err != nil {
		_ = markFileReplicaFailed(fileCode, replicaBackendR2, err.Error())
		log.Printf("upload file to R2 failed: code=%s, key=%s, path=%s, err=%v", fileCode, job.ObjectKey, job.LocalPath, err)
		return
	}

	if err := markFileReplicaUploaded(fileCode, replicaBackendR2); err != nil {
		log.Printf("mark R2 replica as uploaded failed: code=%s, err=%v", fileCode, err)
		return
	}
	if _, err := db.Exec("UPDATE files SET primary_backend = ? WHERE code = ?", replicaBackendR2, fileCode); err != nil {
		log.Printf("update file primary backend after R2 upload failed: code=%s, err=%v", fileCode, err)
		return
	}
	if err := deleteFileReplicaMultipartState(fileCode, replicaBackendR2); err != nil {
		log.Printf("delete R2 multipart state after completion failed: code=%s, err=%v", fileCode, err)
	}

	if err := os.Remove(job.LocalPath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("remove local file after R2 upload failed: code=%s, path=%s, err=%v", fileCode, job.LocalPath, err)
		}
	} else {
		log.Printf("local file removed after R2 upload: code=%s, path=%s", fileCode, job.LocalPath)
	}

	log.Printf("R2 upload completed: code=%s, key=%s", fileCode, job.ObjectKey)
}

func (r *R2Replicator) unmarkQueued(fileCode string) {
	r.mu.Lock()
	delete(r.queued, fileCode)
	r.mu.Unlock()
}

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const (
	r2MultipartMinPartSize          int64 = 5 * 1024 * 1024
	r2MultipartSmallThreshold       int64 = 512 * 1024 * 1024
	r2MultipartMediumThreshold      int64 = 5 * 1024 * 1024 * 1024
	r2MultipartSmallPartSize        int64 = 16 * 1024 * 1024
	r2MultipartMediumPartSize       int64 = 32 * 1024 * 1024
	r2MultipartLargePartSize        int64 = 64 * 1024 * 1024
	r2MultipartMaxParts                   = 10000
	r2MultipartSessionRetryAttempts       = 2
)

func buildR2MultipartPlan(fileSize int64) (int64, int) {
	if fileSize <= 0 {
		return 0, 0
	}

	partSize := r2MultipartMediumPartSize
	switch {
	case fileSize < r2MultipartSmallThreshold:
		partSize = r2MultipartSmallPartSize
	case fileSize < r2MultipartMediumThreshold:
		partSize = r2MultipartMediumPartSize
	default:
		partSize = r2MultipartLargePartSize
	}

	if partSize < r2MultipartMinPartSize {
		partSize = r2MultipartMinPartSize
	}

	totalParts := int((fileSize + partSize - 1) / partSize)
	for totalParts > r2MultipartMaxParts {
		partSize *= 2
		totalParts = int((fileSize + partSize - 1) / partSize)
	}

	return partSize, totalParts
}

func (r *R2Replicator) uploadLocalFileMultipart(fileCode, objectKey, localPath string) error {
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	if fileInfo.Size() == 0 {
		return r.uploadZeroByteFile(objectKey, localPath)
	}

	var lastErr error
	for attempt := 0; attempt < r2MultipartSessionRetryAttempts; attempt++ {
		if attempt > 0 {
			log.Printf("retry R2 multipart upload with new session: code=%s, key=%s, attempt=%d", fileCode, objectKey, attempt+1)
		}

		lastErr = r.uploadLocalFileMultipartOnce(fileCode, objectKey, localPath, fileInfo.Size())
		if lastErr == nil {
			return nil
		}
		if !isNoSuchUploadError(lastErr) {
			return lastErr
		}
		if err := r.abortAndDeleteMultipartState(fileCode, replicaBackendR2); err != nil {
			return fmt.Errorf("%w; reset multipart state failed: %v", lastErr, err)
		}
	}
	return lastErr
}

func (r *R2Replicator) uploadLocalFileMultipartOnce(fileCode, objectKey, localPath string, fileSize int64) error {
	session, err := r.getOrCreateMultipartSession(fileCode, objectKey, fileSize)
	if err != nil {
		return err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	uploadedParts, err := listFileReplicaUploadedParts(fileCode, replicaBackendR2, session.UploadID)
	if err != nil {
		return err
	}
	uploadedLookup := make(map[int32]struct{}, len(uploadedParts))
	for _, part := range uploadedParts {
		uploadedLookup[part.PartNumber] = struct{}{}
	}

	for partNumber := 1; partNumber <= session.TotalParts; partNumber++ {
		partNumber32 := int32(partNumber)
		if _, exists := uploadedLookup[partNumber32]; exists {
			continue
		}

		offset := int64(partNumber-1) * session.PartSize
		partSize := minInt64(session.PartSize, fileSize-offset)
		section := io.NewSectionReader(file, offset, partSize)

		out, err := r.client.UploadPart(context.Background(), &s3.UploadPartInput{
			Bucket:        aws.String(r.cfg.Bucket),
			Key:           aws.String(session.ObjectKey),
			UploadId:      aws.String(session.UploadID),
			PartNumber:    aws.Int32(partNumber32),
			Body:          section,
			ContentLength: aws.Int64(partSize),
		})
		if err != nil {
			return err
		}

		etag := strings.TrimSpace(aws.ToString(out.ETag))
		if etag == "" {
			return fmt.Errorf("empty ETag returned for part %d", partNumber)
		}
		if err := saveFileReplicaUploadedPart(fileCode, replicaBackendR2, session.UploadID, partNumber32, etag, partSize); err != nil {
			return err
		}
	}

	parts, err := listFileReplicaUploadedParts(fileCode, replicaBackendR2, session.UploadID)
	if err != nil {
		return err
	}
	if len(parts) != session.TotalParts {
		return fmt.Errorf("multipart upload incomplete: uploaded parts=%d, expected=%d", len(parts), session.TotalParts)
	}

	completedParts := make([]s3types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		partNumber := part.PartNumber
		etag := part.ETag
		completedParts = append(completedParts, s3types.CompletedPart{
			ETag:       aws.String(etag),
			PartNumber: aws.Int32(partNumber),
		})
	}

	_, err = r.client.CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(r.cfg.Bucket),
		Key:      aws.String(session.ObjectKey),
		UploadId: aws.String(session.UploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	return err
}

func (r *R2Replicator) getOrCreateMultipartSession(fileCode, objectKey string, fileSize int64) (fileReplicaUploadSession, error) {
	partSize, totalParts := buildR2MultipartPlan(fileSize)
	existing, err := loadFileReplicaUploadSession(fileCode, replicaBackendR2)
	if err == nil {
		if existing.ObjectKey == objectKey && existing.PartSize == partSize && existing.TotalParts == totalParts && strings.TrimSpace(existing.UploadID) != "" {
			if err := updateFileReplicaUploadSessionStatus(fileCode, replicaBackendR2, replicaStatusUploading); err != nil {
				return fileReplicaUploadSession{}, err
			}
			existing.Status = replicaStatusUploading
			return existing, nil
		}
		if err := r.abortAndDeleteMultipartState(fileCode, replicaBackendR2); err != nil {
			return fileReplicaUploadSession{}, err
		}
	} else if err != sql.ErrNoRows {
		return fileReplicaUploadSession{}, err
	}

	createOut, err := r.client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(r.cfg.Bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fileReplicaUploadSession{}, err
	}

	session := fileReplicaUploadSession{
		FileCode:   fileCode,
		Backend:    replicaBackendR2,
		ObjectKey:  objectKey,
		UploadID:   aws.ToString(createOut.UploadId),
		PartSize:   partSize,
		TotalParts: totalParts,
		Status:     replicaStatusUploading,
	}
	if strings.TrimSpace(session.UploadID) == "" {
		return fileReplicaUploadSession{}, fmt.Errorf("empty multipart upload ID returned by R2")
	}
	if err := upsertFileReplicaUploadSession(session); err != nil {
		return fileReplicaUploadSession{}, err
	}
	return session, nil
}

func (r *R2Replicator) abortAndDeleteMultipartState(fileCode, backend string) error {
	session, err := loadFileReplicaUploadSession(fileCode, backend)
	if err == nil && strings.TrimSpace(session.UploadID) != "" && strings.TrimSpace(session.ObjectKey) != "" {
		_, abortErr := r.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(r.cfg.Bucket),
			Key:      aws.String(session.ObjectKey),
			UploadId: aws.String(session.UploadID),
		})
		if abortErr != nil && !isNoSuchUploadError(abortErr) {
			return abortErr
		}
	} else if err != nil && err != sql.ErrNoRows {
		return err
	}

	return deleteFileReplicaMultipartState(fileCode, backend)
}

func (r *R2Replicator) uploadZeroByteFile(objectKey, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = r.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(r.cfg.Bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String("application/octet-stream"),
	})
	return err
}

func isNoSuchUploadError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(apiErr.ErrorCode()), "NoSuchUpload")
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

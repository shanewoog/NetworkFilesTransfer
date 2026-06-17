package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const r2DeleteTimeout = 30 * time.Second

func shouldProactivelyDeleteR2Objects() bool {
	return globalR2Cfg.isEnabled() && globalCfg.ExpireHours > 0 && globalCfg.ExpireHours < 24
}

func deleteUploadedR2ObjectForCode(fileCode string) error {
	fileCode = strings.TrimSpace(fileCode)
	if fileCode == "" || !shouldProactivelyDeleteR2Objects() {
		return nil
	}

	objectKey, err := getUploadedReplicaObjectKey(fileCode, replicaBackendR2)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	client, err := getR2ManagementClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), r2DeleteTimeout)
	defer cancel()

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(globalR2Cfg.Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("delete object %q from bucket %q: %w", objectKey, globalR2Cfg.Bucket, err)
	}
	return nil
}

func getR2ManagementClient() (*s3.Client, error) {
	if r2Replicator != nil && r2Replicator.client != nil {
		return r2Replicator.client, nil
	}
	if r2DirectUploader != nil && r2DirectUploader.client != nil {
		return r2DirectUploader.client, nil
	}
	return newR2ManagementClient(globalR2Cfg)
}

func newR2ManagementClient(cfg R2Config) (*s3.Client, error) {
	cfg = cfg.normalized()
	if !cfg.isEnabled() {
		return nil, fmt.Errorf("R2 is disabled")
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

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	}), nil
}

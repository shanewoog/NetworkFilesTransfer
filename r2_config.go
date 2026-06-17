package main

import (
	"net/url"
	"strings"
)

type R2Config struct {
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Region          string `json:"region"`
	Prefix          string `json:"prefix"`
	AccessDomain    string `json:"access_domain"`
}

var globalR2Cfg R2Config

func (cfg R2Config) normalized() R2Config {
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Region == "" {
		cfg.Region = "auto"
	}
	cfg.Prefix = strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	cfg.AccessDomain = strings.TrimRight(strings.TrimSpace(cfg.AccessDomain), "/")
	return cfg
}

func (cfg R2Config) isEnabled() bool {
	return cfg.Enabled
}

func (cfg R2Config) isComplete() bool {
	return cfg.Endpoint != "" &&
		cfg.Bucket != "" &&
		cfg.AccessKeyID != "" &&
		cfg.SecretAccessKey != ""
}

func (cfg R2Config) hasAccessDomain() bool {
	return cfg.isEnabled() && cfg.AccessDomain != ""
}

func (cfg R2Config) buildAccessURL(objectKey string) string {
	if !cfg.hasAccessDomain() {
		return ""
	}

	objectKey = strings.Trim(strings.TrimSpace(objectKey), "/")
	if objectKey == "" {
		return ""
	}

	segments := strings.Split(objectKey, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	return cfg.AccessDomain + "/" + strings.Join(segments, "/")
}

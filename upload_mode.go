package main

import "strings"

const (
	uploadModeLocal         = "local"
	uploadModeLocalThenSync = "local_then_sync"
	uploadModeR2Direct      = "r2_direct"
)

func normalizedConfiguredUploadMode() string {
	mode := strings.TrimSpace(strings.ToLower(globalCfg.UploadMode))
	switch mode {
	case uploadModeLocal, uploadModeLocalThenSync, uploadModeR2Direct:
		return mode
	default:
		return ""
	}
}

func effectiveUploadMode() string {
	if !globalR2Cfg.isEnabled() {
		return uploadModeLocal
	}

	if normalizedConfiguredUploadMode() == uploadModeR2Direct {
		return uploadModeR2Direct
	}

	return uploadModeLocalThenSync
}

func isR2DirectUploadMode() bool {
	return effectiveUploadMode() == uploadModeR2Direct
}

func isLocalUploadMode() bool {
	return effectiveUploadMode() != uploadModeR2Direct
}

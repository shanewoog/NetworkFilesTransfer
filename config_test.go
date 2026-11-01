package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetConfigExposesMaxSingleSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldCfg := globalCfg
	oldVersion := appVersion
	defer func() {
		globalCfg = oldCfg
		appVersion = oldVersion
	}()

	globalCfg = Config{
		Domain:          "https://example.com",
		ExpireHours:     168,
		MaxSingleSizeGB: 15,
	}
	appVersion = "1.0.18"

	router := gin.New()
	router.GET("/api/config", getConfig)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		MaxSingleSizeGB int64 `json:"max_single_size_gb"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.MaxSingleSizeGB != 15 {
		t.Fatalf("max_single_size_gb = %d, want 15", payload.MaxSingleSizeGB)
	}
}

func TestIndexSingleSizeTextIsNotHardcoded(t *testing.T) {
	content, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read index.html failed: %v", err)
	}

	html := string(content)
	if strings.Contains(html, "只支持最大 10GB 单文件上传") {
		t.Fatalf("index.html still hardcodes 10GB single-file limit")
	}
	if strings.Contains(html, "single-file uploads up to 10 GB") {
		t.Fatalf("index.html still hardcodes 10 GB single-file limit")
	}
}

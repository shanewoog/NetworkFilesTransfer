package main

import "testing"

func TestNormalizeStoredVersionStripsUTF8BOM(t *testing.T) {
	got := normalizeStoredVersion("\uFEFF1.0.9")
	if got != "1.0.9" {
		t.Fatalf("normalizeStoredVersion() = %q, want %q", got, "1.0.9")
	}
}

func TestShouldAutoBumpVersionSkipsGoRunTempBinary(t *testing.T) {
	exePath := `C:\Users\shane\AppData\Local\Temp\go-build123456789\b001\exe\NetworkFilesTransfer.exe`
	if shouldAutoBumpVersion(exePath) {
		t.Fatalf("shouldAutoBumpVersion(%q) = true, want false", exePath)
	}
}

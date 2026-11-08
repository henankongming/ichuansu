package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testServerToken() []byte {
	return bytes.Repeat([]byte{0x5a}, 32)
}

func TestHealthRequiresApplicationToken(t *testing.T) {
	handler := NewHandler(t.TempDir(), testServerToken())
	request := httptest.NewRequest(http.MethodGet, HealthPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized health status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, HealthPath, nil)
	request.Header.Set("Authorization", "Bearer "+hex.EncodeToString(testServerToken()))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized health status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("ichuansu")) {
		t.Fatalf("unexpected health response: %s", response.Body.String())
	}
}

func TestUploadVerifiesHashAndRejectsConflict(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(dir, testServerToken())
	payload := []byte("authenticated upload payload")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])

	makeRequest := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, UploadPath+"?name=hello.txt", bytes.NewReader(payload))
		request.ContentLength = int64(len(payload))
		request.Header.Set("Authorization", "Bearer "+hex.EncodeToString(testServerToken()))
		request.Header.Set("X-Ichuansu-SHA256", hash)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := makeRequest()
	if response.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}
	stored, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatal("uploaded content mismatch")
	}

	response = makeRequest()
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate upload status=%d want=%d", response.Code, http.StatusConflict)
	}
}

func TestUploadRejectsBadHashAndTraversal(t *testing.T) {
	handler := NewHandler(t.TempDir(), testServerToken())
	payload := []byte("payload")
	token := "Bearer " + hex.EncodeToString(testServerToken())

	request := httptest.NewRequest(http.MethodPost, UploadPath+"?name=safe.txt", bytes.NewReader(payload))
	request.ContentLength = int64(len(payload))
	request.Header.Set("Authorization", token)
	request.Header.Set("X-Ichuansu-SHA256", hex.EncodeToString(make([]byte, 32)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad hash status=%d want=%d", response.Code, http.StatusUnprocessableEntity)
	}

	request = httptest.NewRequest(http.MethodPost, UploadPath+"?name=..%2fescape.txt", bytes.NewReader(payload))
	request.ContentLength = int64(len(payload))
	request.Header.Set("Authorization", token)
	request.Header.Set("X-Ichuansu-SHA256", hex.EncodeToString(sha256.New().Sum(nil)))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("traversal status=%d want=%d", response.Code, http.StatusBadRequest)
	}
}

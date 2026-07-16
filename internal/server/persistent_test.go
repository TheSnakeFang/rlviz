package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersistentHandlerRegistersAuthenticatedSource(t *testing.T) {
	called := false
	handler := NewPersistentHandler(nil, "secret", func(_ context.Context, path, adapter string) (Registration, error) {
		called = true
		return Registration{SourceID: "source", Path: path, URL: "/?trajectory=source&indexed=1"}, nil
	}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"path":"trace","adapter":"plugin"}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !called {
		t.Fatalf("status=%d body=%s called=%v", response.Code, response.Body.String(), called)
	}
	var registration Registration
	if err := json.NewDecoder(response.Body).Decode(&registration); err != nil {
		t.Fatal(err)
	}
	if registration.SourceID != "source" || registration.URL != "/?trajectory=source&indexed=1" {
		t.Fatalf("registration = %#v", registration)
	}
}

func TestPersistentHandlerProtectsRegistrationAndIndexedReads(t *testing.T) {
	handler := NewPersistentHandler(nil, "secret", nil, nil)
	for _, target := range []string{"/api/v1/sources", "/api/v1/indexed/events?trajectory=source"} {
		method := http.MethodGet
		if target == "/api/v1/sources" {
			method = http.MethodPost
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d", target, response.Code)
		}
	}
}

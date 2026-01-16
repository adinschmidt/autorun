package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"autorun/internal/models"
)

func TestRouter_ServiceAction_RequiresName(t *testing.T) {
	provider := &fakeProvider{}
	router := NewRouter(provider, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/services/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestRouter_ServiceAction_ParsesNameAndDefaultsScopeUser(t *testing.T) {
	provider := &fakeProvider{}
	router := NewRouter(provider, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/services/com.example.demo/start", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if len(provider.startCalls) != 1 {
		t.Fatalf("expected 1 Start call, got %d", len(provider.startCalls))
	}
	if provider.startCalls[0].name != "com.example.demo" {
		t.Fatalf("expected service name %q, got %q", "com.example.demo", provider.startCalls[0].name)
	}
	if provider.startCalls[0].scope != models.ScopeUser {
		t.Fatalf("expected default scope %q, got %q", models.ScopeUser, provider.startCalls[0].scope)
	}
}

func TestRouter_ServiceAction_ParsesScopeSystem(t *testing.T) {
	provider := &fakeProvider{}
	router := NewRouter(provider, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/services/com.example.demo/start?scope=system", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if len(provider.startCalls) != 1 {
		t.Fatalf("expected 1 Start call, got %d", len(provider.startCalls))
	}
	if provider.startCalls[0].scope != models.ScopeSystem {
		t.Fatalf("expected scope %q, got %q", models.ScopeSystem, provider.startCalls[0].scope)
	}
}

func TestRouter_ServiceAction_UnknownAction(t *testing.T) {
	provider := &fakeProvider{}
	router := NewRouter(provider, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/services/com.example.demo/unknown-action", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

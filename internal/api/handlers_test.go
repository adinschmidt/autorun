package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"autorun/internal/models"
)

func TestParseScope_DefaultsToUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	if got := parseScope(req); got != models.ScopeUser {
		t.Fatalf("expected %q, got %q", models.ScopeUser, got)
	}
}

func TestParseScope_System(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/services?scope=system", nil)
	if got := parseScope(req); got != models.ScopeSystem {
		t.Fatalf("expected %q, got %q", models.ScopeSystem, got)
	}
}

func TestParseScope_User(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/services?scope=user", nil)
	if got := parseScope(req); got != models.ScopeUser {
		t.Fatalf("expected %q, got %q", models.ScopeUser, got)
	}
}

func TestListServices_ScopeAll_Default(t *testing.T) {
	provider := &fakeProvider{
		systemServices: []models.Service{{Name: "sys", Scope: models.ScopeSystem}},
		userServices:   []models.Service{{Name: "usr", Scope: models.ScopeUser}},
	}
	h := NewHandler(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	rr := httptest.NewRecorder()
	h.ListServices(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if len(provider.listCalls) != 2 {
		t.Fatalf("expected 2 ListServices calls, got %d", len(provider.listCalls))
	}
	if provider.listCalls[0] != models.ScopeSystem {
		t.Fatalf("expected first scope %q, got %q", models.ScopeSystem, provider.listCalls[0])
	}
	if provider.listCalls[1] != models.ScopeUser {
		t.Fatalf("expected second scope %q, got %q", models.ScopeUser, provider.listCalls[1])
	}
}

func TestListServices_ScopeAll_Explicit(t *testing.T) {
	provider := &fakeProvider{}
	h := NewHandler(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/services?scope=all", nil)
	rr := httptest.NewRecorder()
	h.ListServices(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if len(provider.listCalls) != 2 {
		t.Fatalf("expected 2 ListServices calls, got %d", len(provider.listCalls))
	}
}

func TestListServices_ScopeUser_OnlyOneProviderCall(t *testing.T) {
	provider := &fakeProvider{}
	h := NewHandler(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/services?scope=user", nil)
	rr := httptest.NewRecorder()
	h.ListServices(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if len(provider.listCalls) != 1 {
		t.Fatalf("expected 1 ListServices call, got %d", len(provider.listCalls))
	}
	if provider.listCalls[0] != models.ScopeUser {
		t.Fatalf("expected scope %q, got %q", models.ScopeUser, provider.listCalls[0])
	}
}

func TestExtractServiceName(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "plain name", path: "/api/services/foo", want: "foo"},
		{name: "name with action", path: "/api/services/foo/start", want: "foo"},
		{name: "already trimmed", path: "foo/start", want: "foo"},
		{name: "empty", path: "", want: ""},
		{name: "prefix only", path: "/api/services/", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractServiceName(tc.path); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

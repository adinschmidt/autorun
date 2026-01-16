package api

import (
	"context"

	"autorun/internal/models"
)

type fakeProvider struct {
	name string

	systemServices []models.Service
	userServices   []models.Service

	listCalls  []models.Scope
	getCalls   []getCall
	startCalls []serviceCall
}

type serviceCall struct {
	name  string
	scope models.Scope
}

type getCall struct {
	name  string
	scope models.Scope
}

func (p *fakeProvider) Name() string {
	if p.name == "" {
		return "fake"
	}
	return p.name
}

func (p *fakeProvider) ListServices(scope models.Scope) ([]models.Service, error) {
	p.listCalls = append(p.listCalls, scope)
	if scope == models.ScopeSystem {
		return append([]models.Service(nil), p.systemServices...), nil
	}
	return append([]models.Service(nil), p.userServices...), nil
}

func (p *fakeProvider) GetService(name string, scope models.Scope) (*models.Service, error) {
	p.getCalls = append(p.getCalls, getCall{name: name, scope: scope})
	return &models.Service{Name: name, Scope: scope}, nil
}

func (p *fakeProvider) Start(name string, scope models.Scope) error {
	p.startCalls = append(p.startCalls, serviceCall{name: name, scope: scope})
	return nil
}

func (p *fakeProvider) Stop(name string, scope models.Scope) error    { return nil }
func (p *fakeProvider) Restart(name string, scope models.Scope) error { return nil }
func (p *fakeProvider) Enable(name string, scope models.Scope) error  { return nil }
func (p *fakeProvider) Disable(name string, scope models.Scope) error { return nil }

func (p *fakeProvider) StreamLogs(ctx context.Context, name string, scope models.Scope) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (p *fakeProvider) CreateService(config models.ServiceConfig, scope models.Scope) error {
	return nil
}

func (p *fakeProvider) DeleteService(name string, scope models.Scope) error {
	return nil
}

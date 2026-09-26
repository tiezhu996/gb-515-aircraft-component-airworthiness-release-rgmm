package service

import (
	"context"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/config"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
)

// noopSecurity is a SecurityService stub for service-level tests where audit
// persistence is verified directly on the repository database instead.
type noopSecurity struct{}

func (noopSecurity) Login(context.Context, dto.LoginRequest) (dto.LoginResponse, error) {
	return dto.LoginResponse{}, nil
}
func (noopSecurity) Audit(context.Context, string, string, string, string, uint, string, string, string) error {
	return nil
}
func (noopSecurity) ListAudits(context.Context, int, int, string) ([]model.AuditLog, int64, error) {
	return nil, 0, nil
}
func (noopSecurity) AuditSummary(context.Context, time.Duration) (model.AuditSummary, error) {
	return model.AuditSummary{}, nil
}
func (noopSecurity) EntityHistory(context.Context, string, uint, int) ([]model.AuditLog, error) {
	return nil, nil
}
func (noopSecurity) RuntimeConfig() config.PublicConfig { return config.PublicConfig{} }

var _ SecurityService = noopSecurity{}

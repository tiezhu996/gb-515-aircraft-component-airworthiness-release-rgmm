package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"gorm.io/gorm"
)

type AircraftPartService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.AircraftPart], error)
	Get(context.Context, uint) (model.AircraftPart, error)
	Create(context.Context, dto.CreateAircraftPart, string, string) (model.AircraftPart, error)
	Update(context.Context, uint, dto.UpdateAircraftPart, string, string) (model.AircraftPart, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.AircraftPart, error)
	Install(context.Context, uint, dto.InstallAircraftPart, string, string) (model.AircraftPart, error)
	Uninstall(context.Context, uint, dto.UninstallAircraftPart, string, string) (model.AircraftPart, error)
	ListInstallations(context.Context, uint) ([]model.PartInstallation, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type aircraftPartService struct {
	repository repository.AircraftPartRepository
	security   SecurityService
}

func NewAircraftPartService(repo repository.AircraftPartRepository, security SecurityService) AircraftPartService {
	return &aircraftPartService{repository: repo, security: security}
}

func (s *aircraftPartService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.AircraftPart], error) {
	return s.repository.List(ctx, query)
}

func (s *aircraftPartService) Get(ctx context.Context, id uint) (model.AircraftPart, error) {
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) Create(ctx context.Context, input dto.CreateAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	if err := validateAircraftPartBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.AircraftPart{}, err
	}
	item := model.AircraftPart{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.AircraftPartInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.AircraftPart{}, fmt.Errorf("create 航空部件: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "AircraftPart", item.ID, "", item.Status, "created 航空部件")
	return item, nil
}

func (s *aircraftPartService) Update(ctx context.Context, id uint, input dto.UpdateAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	if err := validateAircraftPartBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.AircraftPart{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.AircraftPart{}, fmt.Errorf("update 航空部件: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "AircraftPart", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.AircraftPartTransitions, current.Status, target) {
		return model.AircraftPart{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.AircraftPart{}, fmt.Errorf("transition 航空部件: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "AircraftPart", id, before, target, input.Reason); err != nil {
		return model.AircraftPart{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

// Install registers the 装机履历 (机型/架次/安装位置/装机人) and moves a released
// part to installed. A second active record for the same part, or another part
// already occupying the 架次/安装位置 slot, rejects the request with a conflict
// error that names the blocking record.
func (s *aircraftPartService) Install(ctx context.Context, id uint, input dto.InstallAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	aircraftModel := strings.TrimSpace(input.AircraftModel)
	aircraftTail := strings.ToUpper(strings.TrimSpace(input.AircraftTail))
	position := strings.TrimSpace(input.Position)
	installer := strings.TrimSpace(input.Installer)
	if aircraftModel == "" || aircraftTail == "" || position == "" || installer == "" {
		return model.AircraftPart{}, ErrInvalidInput
	}
	if active, lookupErr := s.repository.ActiveInstallationForPart(ctx, id); lookupErr == nil {
		return model.AircraftPart{}, fmt.Errorf("%w: 部件 %s 已存在在装记录 #%d（架次 %s / 位置 %s / 装机人 %s）",
			ErrInstallationBlocked, current.Code, active.ID, active.AircraftTail, active.Position, active.Installer)
	} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return model.AircraftPart{}, lookupErr
	}
	if current.Status != string(constants.PartStateReleased) {
		return model.AircraftPart{}, fmt.Errorf("%w: %s -> installed", ErrInvalidTransition, current.Status)
	}
	if active, lookupErr := s.repository.ActiveInstallationForSlot(ctx, aircraftTail, position); lookupErr == nil {
		return model.AircraftPart{}, fmt.Errorf("%w: 架次 %s 位置 %s 已被装机记录 #%d 占用（部件 %s / 装机人 %s）",
			ErrInstallationBlocked, aircraftTail, position, active.ID, active.PartCode, active.Installer)
	} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return model.AircraftPart{}, lookupErr
	}
	now := time.Now().UTC()
	partKey := "PART:" + current.Code
	slotKey := "SLOT:" + aircraftTail + "|" + position
	installation := model.PartInstallation{
		PartID: id, PartCode: current.Code, AircraftModel: aircraftModel, AircraftTail: aircraftTail,
		Position: position, Installer: installer, InstalledBy: actor, InstalledAt: now, Active: true,
		ActivePartKey: &partKey, ActiveSlotKey: &slotKey,
	}
	current.Status = string(constants.PartStateInstalled)
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = now
	detail := fmt.Sprintf("installed on %s %s position %s by %s", aircraftModel, aircraftTail, position, installer)
	if err := s.repository.Install(ctx, id, input.ExpectedVersion, &current, &installation, actor, requestID, detail); err != nil {
		if isUniqueViolation(err) {
			return model.AircraftPart{}, s.installationConflictDetails(ctx, id, aircraftTail, position)
		}
		return model.AircraftPart{}, fmt.Errorf("install 航空部件: %w", err)
	}
	return s.repository.Get(ctx, id)
}

// Uninstall closes the active installation with a mandatory reason and returns
// the part to inspection. The historical record is kept, never deleted.
func (s *aircraftPartService) Uninstall(ctx context.Context, id uint, input dto.UninstallAircraftPart, actor, requestID string) (model.AircraftPart, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.AircraftPart{}, err
	}
	if current.Status != string(constants.PartStateInstalled) {
		return model.AircraftPart{}, fmt.Errorf("%w: %s -> inspection", ErrInvalidTransition, current.Status)
	}
	reason := strings.TrimSpace(input.Reason)
	if len(reason) < 3 {
		return model.AircraftPart{}, ErrInvalidInput
	}
	installation, err := s.repository.ActiveInstallationForPart(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AircraftPart{}, fmt.Errorf("%w: 部件 %s", ErrNoActiveInstallation, current.Code)
		}
		return model.AircraftPart{}, err
	}
	now := time.Now().UTC()
	installation.RemovedAt = &now
	installation.RemovedBy = actor
	installation.RemovalReason = reason
	current.Status = string(constants.PartStateInspection)
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = now
	detail := fmt.Sprintf("uninstalled from %s %s position %s: %s",
		installation.AircraftModel, installation.AircraftTail, installation.Position, reason)
	if err := s.repository.Uninstall(ctx, id, input.ExpectedVersion, &current, &installation, actor, requestID, detail); err != nil {
		return model.AircraftPart{}, fmt.Errorf("uninstall 航空部件: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *aircraftPartService) ListInstallations(ctx context.Context, id uint) ([]model.PartInstallation, error) {
	if _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repository.ListInstallations(ctx, id)
}

func (s *aircraftPartService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "AircraftPart", id, current.Status, "deleted", "soft deleted 航空部件")
}

func (s *aircraftPartService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateAircraftPartBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}

// isUniqueViolation normalizes the driver-specific unique-key errors raised by
// the ActivePartKey/ActiveSlotKey sentinels (MySQL 1062, SQLite, PostgreSQL).
func isUniqueViolation(err error) bool {
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") ||
		strings.Contains(message, "UNIQUE constraint failed") ||
		strings.Contains(message, "duplicate key")
}

// installationConflictDetails runs after a unique sentinel rejected a racing
// install and reports which record now holds the part or the slot.
func (s *aircraftPartService) installationConflictDetails(ctx context.Context, partID uint, aircraftTail, position string) error {
	if active, err := s.repository.ActiveInstallationForPart(ctx, partID); err == nil {
		return fmt.Errorf("%w: 部件已存在在装记录 #%d（架次 %s / 位置 %s / 装机人 %s）",
			ErrInstallationBlocked, active.ID, active.AircraftTail, active.Position, active.Installer)
	}
	if active, err := s.repository.ActiveInstallationForSlot(ctx, aircraftTail, position); err == nil {
		return fmt.Errorf("%w: 架次 %s 位置 %s 已被装机记录 #%d 占用（部件 %s / 装机人 %s）",
			ErrInstallationBlocked, aircraftTail, position, active.ID, active.PartCode, active.Installer)
	}
	return ErrInstallationBlocked
}

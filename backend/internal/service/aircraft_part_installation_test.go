package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/config"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAircraftPartInstallationLifecycle(t *testing.T) {
	db := newInstallationTestDB(t)
	repo := repository.NewAircraftPartRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	service := NewAircraftPartService(repo, security)
	ctx := context.Background()

	part := seedInstallablePart(t, repo, "AP-INSTALL-1", "released")
	other := seedInstallablePart(t, repo, "AP-INSTALL-2", "released")
	inInspection := seedInstallablePart(t, repo, "AP-INSTALL-3", "inspection")

	if _, err := service.Install(ctx, inInspection.ID, installInput(inInspection.Version, "B-9009", "右发吊舱"), "operator", "install-wrong-state"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("only released parts can be installed, got %v", err)
	}

	installed, err := service.Install(ctx, part.ID, installInput(part.Version, "B-9001", "左发吊舱"), "operator", "install-1")
	if err != nil {
		t.Fatalf("install released part: %v", err)
	}
	if installed.Status != "installed" || installed.Version != part.Version+1 {
		t.Fatalf("unexpected installed part: %#v", installed)
	}
	if len(installed.Installations) != 1 {
		t.Fatalf("expected one active installation record, got %#v", installed.Installations)
	}
	active := installed.Installations[0]
	if !active.Active || active.AircraftModel != "ARJ21-700" || active.AircraftTail != "B-9001" ||
		active.Position != "左发吊舱" || active.Installer != "张工" || active.InstalledBy != "operator" {
		t.Fatalf("installation record missing registration fields: %#v", active)
	}

	if _, err := service.Install(ctx, part.ID, installInput(installed.Version, "B-9002", "右发吊舱"), "operator", "install-duplicate"); !errors.Is(err, ErrInstallationBlocked) {
		t.Fatalf("duplicate install of the same part must be blocked, got %v", err)
	} else if !strings.Contains(err.Error(), "在装记录 #") {
		t.Fatalf("duplicate install error must name the blocking record, got %v", err)
	}

	if _, err := service.Install(ctx, other.ID, installInput(other.Version, "B-9001", "左发吊舱"), "operator", "install-slot-conflict"); !errors.Is(err, ErrInstallationBlocked) {
		t.Fatalf("occupied slot must block the install, got %v", err)
	} else if !strings.Contains(err.Error(), "AP-INSTALL-1") {
		t.Fatalf("slot conflict error must name the occupying record, got %v", err)
	}

	for _, target := range []string{"inspection", "hold"} {
		blocked := dto.TransitionRequest{Status: target, ExpectedVersion: installed.Version, Reason: "installed parts must be uninstalled first"}
		if _, err := service.Transition(ctx, part.ID, blocked, "operator", "transition-installed-"+target); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("installed part must not transition to %s, got %v", target, err)
		}
	}

	if _, err := service.Uninstall(ctx, part.ID, dto.UninstallAircraftPart{ExpectedVersion: installed.Version, Reason: "  "}, "operator", "uninstall-no-reason"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("uninstall without reason must be rejected, got %v", err)
	}

	uninstalled, err := service.Uninstall(ctx, part.ID, dto.UninstallAircraftPart{ExpectedVersion: installed.Version, Reason: "定检到期拆下复查"}, "operator", "uninstall-1")
	if err != nil {
		t.Fatalf("uninstall installed part: %v", err)
	}
	if uninstalled.Status != "inspection" || uninstalled.Version != installed.Version+1 {
		t.Fatalf("unexpected uninstalled part: %#v", uninstalled)
	}
	if len(uninstalled.Installations) != 1 {
		t.Fatalf("uninstall must keep the historical record, got %#v", uninstalled.Installations)
	}
	removed := uninstalled.Installations[0]
	if removed.Active || removed.RemovedAt == nil || removed.RemovedBy != "operator" || removed.RemovalReason != "定检到期拆下复查" {
		t.Fatalf("removal details missing on historical record: %#v", removed)
	}

	if _, err := service.Install(ctx, other.ID, installInput(other.Version, "B-9001", "左发吊舱"), "operator", "install-after-free"); err != nil {
		t.Fatalf("slot must be installable again after uninstall: %v", err)
	}

	released, err := service.Transition(ctx, part.ID, dto.TransitionRequest{
		Status: "released", ExpectedVersion: uninstalled.Version, Reason: "复查通过重新放行",
	}, "operator", "transition-re-release")
	if err != nil {
		t.Fatalf("uninstalled part must be releasable again: %v", err)
	}
	reinstalled, err := service.Install(ctx, part.ID, installInput(released.Version, "B-9003", "中央油箱"), "operator", "install-2")
	if err != nil {
		t.Fatalf("released part must be installable again: %v", err)
	}
	if reinstalled.Status != "installed" {
		t.Fatalf("unexpected reinstalled part: %#v", reinstalled)
	}

	history, err := service.ListInstallations(ctx, part.ID)
	if err != nil {
		t.Fatalf("list installations: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history must keep every install record, got %#v", history)
	}
	if !history[0].Active || history[1].Active || history[1].RemovalReason == "" {
		t.Fatalf("history must show the active record first and keep the closed one, got %#v", history)
	}
	assertAuditActions(t, db, part.ID, "install", "uninstall", "transition")
}

func TestAircraftPartUninstallRequiresActiveRecord(t *testing.T) {
	db := newInstallationTestDB(t)
	repo := repository.NewAircraftPartRepository(db)
	service := NewAircraftPartService(repo, nil)
	ctx := context.Background()

	released := seedInstallablePart(t, repo, "AP-INSTALL-4", "released")
	if _, err := service.Uninstall(ctx, released.ID, dto.UninstallAircraftPart{ExpectedVersion: released.Version, Reason: "尚未装机"}, "operator", "uninstall-not-installed"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("only installed parts can be uninstalled, got %v", err)
	}

	orphan := seedInstallablePart(t, repo, "AP-INSTALL-5", "installed")
	if _, err := service.Uninstall(ctx, orphan.ID, dto.UninstallAircraftPart{ExpectedVersion: orphan.Version, Reason: "记录在案缺失"}, "operator", "uninstall-orphan"); !errors.Is(err, ErrNoActiveInstallation) {
		t.Fatalf("installed part without active record must be reported, got %v", err)
	}
}

func newInstallationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}, &model.AircraftPart{}, &model.PartInstallation{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func seedInstallablePart(t *testing.T, repo repository.AircraftPartRepository, code, status string) model.AircraftPart {
	t.Helper()
	item := model.AircraftPart{
		BaseModel: model.BaseModel{Code: code, Name: "装机测试部件 " + code, Status: status, Version: 1},
		Facility:  "装机验证机库", Owner: "维修一组", Category: "发动机", RiskLevel: "high",
		MetricValue: 100, MetricUnit: "percent", EffectiveAt: time.Now().UTC(),
		Evidence: "放行证据齐全", RelatedCode: "REL-INSTALL",
	}
	if err := repo.Create(context.Background(), &item); err != nil {
		t.Fatalf("seed part: %v", err)
	}
	return item
}

func installInput(version uint, tail, position string) dto.InstallAircraftPart {
	return dto.InstallAircraftPart{
		ExpectedVersion: version, AircraftModel: "ARJ21-700",
		AircraftTail: tail, Position: position, Installer: "张工",
	}
}

func assertAuditActions(t *testing.T, db *gorm.DB, partID uint, actions ...string) {
	t.Helper()
	logs := make([]model.AuditLog, 0)
	if err := db.Where("entity_type = ? AND entity_id = ?", "AircraftPart", partID).Find(&logs).Error; err != nil {
		t.Fatalf("list audits: %v", err)
	}
	recorded := make(map[string]bool, len(logs))
	for _, log := range logs {
		recorded[log.Action] = true
		if log.Actor != "operator" {
			t.Fatalf("audit actor must be attributed, got %#v", log)
		}
	}
	for _, action := range actions {
		if !recorded[action] {
			t.Fatalf("expected %s audit for part %d, got %#v", action, partID, logs)
		}
	}
}

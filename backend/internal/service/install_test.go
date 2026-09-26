package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInstallUninstallLifecycleAndConflicts(t *testing.T) {
	db := newInstallTestDB(t)
	repo := repository.NewAircraftPartRepository(db)
	svc := NewAircraftPartService(repo, noopSecurity{})
	ctx := context.Background()

	partA := createPartForInstall(t, ctx, svc, "INST-001")
	partB := createPartForInstall(t, ctx, svc, "INST-002")
	// Move both parts to released along the standard state machine.
	releasePart(t, ctx, svc, partA.ID, 1)
	releasePart(t, ctx, svc, partB.ID, 1)
	releasedA, err := svc.Get(ctx, partA.ID)
	if err != nil {
		t.Fatalf("reload part A: %v", err)
	}
	releasedB, err := svc.Get(ctx, partB.ID)
	if err != nil {
		t.Fatalf("reload part B: %v", err)
	}

	install := dto.InstallAircraftPart{
		ExpectedVersion: releasedA.Version, AircraftModel: "C919", AircraftSerial: "B-001",
		Location: "左翼-1号挂点", InstalledBy: "operator",
	}
	installed, err := svc.Install(ctx, releasedA.ID, install, "operator", "install-1")
	if err != nil {
		t.Fatalf("install part A: %v", err)
	}
	if installed.Status != string(constants.PartStateInstalled) || installed.Version != releasedA.Version+1 {
		t.Fatalf("unexpected installed part: status=%s version=%d", installed.Status, installed.Version)
	}
	if len(installed.InstallRecords) != 1 {
		t.Fatalf("expected one install record, got %d", len(installed.InstallRecords))
	}
	record := installed.InstallRecords[0]
	if record.AircraftModel != "C919" || record.AircraftSerial != "B-001" || record.Location != "左翼-1号挂点" ||
		record.InstalledBy != "operator" || record.InstallRequest != "install-1" || record.RemovedAt != nil {
		t.Fatalf("unexpected install record: %#v", record)
	}

	// The same part must not be installed a second time while mounted.
	_, err = svc.Install(ctx, installed.ID, dto.InstallAircraftPart{
		ExpectedVersion: installed.Version, AircraftModel: "C919", AircraftSerial: "B-002",
		Location: "右翼-2号挂点", InstalledBy: "operator",
	}, "operator", "install-duplicate-part")
	var partConflict *repository.PartInstalledConflict
	if !errors.As(err, &partConflict) {
		t.Fatalf("expected PartInstalledConflict, got %v", err)
	}
	if partConflict.Blocker.RecordID != record.ID || partConflict.Blocker.PartCode != "INST-001" ||
		partConflict.Blocker.AircraftSerial != "B-001" || partConflict.Blocker.Location != "左翼-1号挂点" {
		t.Fatalf("blocker must identify the existing record: %#v", partConflict.Blocker)
	}

	// Another part cannot take the same (架次, 安装位置) slot.
	_, err = svc.Install(ctx, releasedB.ID, dto.InstallAircraftPart{
		ExpectedVersion: releasedB.Version, AircraftModel: "C919", AircraftSerial: "B-001",
		Location: "左翼-1号挂点", InstalledBy: "operator",
	}, "operator", "install-duplicate-slot")
	var slotConflict *repository.SlotOccupiedConflict
	if !errors.As(err, &slotConflict) {
		t.Fatalf("expected SlotOccupiedConflict, got %v", err)
	}
	if slotConflict.Blocker.RecordID != record.ID || slotConflict.Blocker.PartCode != "INST-001" ||
		slotConflict.Blocker.PartID != installed.ID || slotConflict.Blocker.InstalledBy != "operator" {
		t.Fatalf("slot blocker must identify the occupying record: %#v", slotConflict.Blocker)
	}

	// A different slot on the same aircraft is fine for part B.
	installedB, err := svc.Install(ctx, releasedB.ID, dto.InstallAircraftPart{
		ExpectedVersion: releasedB.Version, AircraftModel: "C919", AircraftSerial: "B-001",
		Location: "右翼-2号挂点", InstalledBy: "operator",
	}, "operator", "install-2")
	if err != nil {
		t.Fatalf("install part B in free slot: %v", err)
	}

	// Generic transitions must not advance an installed part toward inspection/hold.
	if _, err := svc.Transition(ctx, installed.ID, dto.TransitionRequest{
		Status: "inspection", ExpectedVersion: installed.Version,
	}, "operator", "bypass-uninstall"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("installed part must not leave via generic transition, got %v", err)
	}
	// Installing through the generic transition endpoint is also rejected.
	if _, err := svc.Transition(ctx, installedB.ID, dto.TransitionRequest{
		Status: "installed", ExpectedVersion: installedB.Version,
	}, "operator", "generic-install"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("installed target must require dedicated endpoint, got %v", err)
	}

	// Uninstall closes the record, returns the part to inspection and keeps history.
	uninstalled, err := svc.Uninstall(ctx, installed.ID, dto.UninstallAircraftPart{
		ExpectedVersion: installed.Version, Reason: "定检到期拆下",
	}, "operator", "uninstall-1")
	if err != nil {
		t.Fatalf("uninstall part A: %v", err)
	}
	if uninstalled.Status != string(constants.PartStateInspection) || uninstalled.Version != installed.Version+1 {
		t.Fatalf("unexpected uninstalled part: status=%s version=%d", uninstalled.Status, uninstalled.Version)
	}
	closed := uninstalled.InstallRecords[0]
	if closed.RemovedAt == nil || closed.RemovedBy != "operator" || closed.RemoveReason != "定检到期拆下" ||
		closed.RemoveRequestID != "uninstall-1" {
		t.Fatalf("install record must carry uninstall attribution: %#v", closed)
	}

	// After uninstall the slot is free for another part, and the part can be reinstalled.
	inspectionA, err := svc.Get(ctx, installed.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := svc.Transition(ctx, inspectionA.ID, dto.TransitionRequest{
		Status: "released", ExpectedVersion: inspectionA.Version, Reason: "re-inspection passed",
	}, "operator", "release-again"); err != nil {
		t.Fatalf("re-release after uninstall: %v", err)
	}
	again, err := svc.Get(ctx, installed.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	reinstalled, err := svc.Install(ctx, again.ID, dto.InstallAircraftPart{
		ExpectedVersion: again.Version, AircraftModel: "C919", AircraftSerial: "B-003",
		Location: "尾翼-3号挂点", InstalledBy: "reviewer",
	}, "reviewer", "install-3")
	if err != nil {
		t.Fatalf("reinstall part A: %v", err)
	}
	if len(reinstalled.InstallRecords) != 2 {
		t.Fatalf("history must retain closed record and append new open record: %#v", reinstalled.InstallRecords)
	}
	open := reinstalled.InstallRecords[0]
	closed2 := reinstalled.InstallRecords[1]
	if open.RemovedAt != nil || open.AircraftSerial != "B-003" {
		t.Fatalf("newest record must be open on B-003: %#v", open)
	}
	if closed2.AircraftSerial != "B-001" || closed2.RemovedAt == nil {
		t.Fatalf("original record must remain and stay closed: %#v", closed2)
	}

	// Uninstall without an active record must fail.
	if _, err := svc.Uninstall(ctx, installedB.ID, dto.UninstallAircraftPart{
		ExpectedVersion: installedB.Version, Reason: "",
	}, "operator", "uninstall-no-reason"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing reason must be rejected, got %v", err)
	}

	assertInstallAuditActions(t, db, installed.ID, []string{"install", "uninstall", "install"})
	assertInstallAuditActions(t, db, installedB.ID, []string{"install"})
}

func TestInstallRequiresReleasedPart(t *testing.T) {
	db := newInstallTestDB(t)
	svc := NewAircraftPartService(repository.NewAircraftPartRepository(db), noopSecurity{})
	ctx := context.Background()
	part := createPartForInstall(t, ctx, svc, "INST-HOLD-1")
	// part stays in received status
	_, err := svc.Install(ctx, part.ID, dto.InstallAircraftPart{
		ExpectedVersion: 1, AircraftModel: "C919", AircraftSerial: "B-100", Location: "挂点-A", InstalledBy: "operator",
	}, "operator", "install-not-released")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("installing a non-released part must be rejected, got %v", err)
	}
}

func newInstallTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AircraftPart{}, &model.InstallRecord{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createPartForInstall(t *testing.T, ctx context.Context, svc AircraftPartService, code string) model.AircraftPart {
	t.Helper()
	part, err := svc.Create(ctx, dto.CreateAircraftPart{
		Code: code, Name: "装机测试部件 " + code, Facility: "机库1", Owner: "运行一组", Category: "发动机",
		RiskLevel: "high", MetricValue: 1, MetricUnit: "unit", EffectiveAt: time.Now().UTC(),
		Evidence: "evidence", RelatedCode: "",
	}, "operator", "create-"+code)
	if err != nil {
		t.Fatalf("create part %s: %v", code, err)
	}
	return part
}

func releasePart(t *testing.T, ctx context.Context, svc AircraftPartService, id, version uint) {
	t.Helper()
	path := []string{"inspection", "released"}
	current := version
	for _, target := range path {
		if _, err := svc.Transition(ctx, id, dto.TransitionRequest{
			Status: target, ExpectedVersion: current, Reason: "release preparation",
		}, "operator", "transition-"+target); err != nil {
			t.Fatalf("transition %s: %v", target, err)
		}
		current++
	}
}

func assertInstallAuditActions(t *testing.T, db *gorm.DB, partID uint, expected []string) {
	t.Helper()
	var audits []model.AuditLog
	if err := db.Where("entity_type = ? AND entity_id = ? AND action IN ?", "AircraftPart", partID, []string{"install", "uninstall"}).
		Order("id").Find(&audits).Error; err != nil {
		t.Fatalf("query install audits: %v", err)
	}
	if len(audits) != len(expected) {
		t.Fatalf("expected %d install/uninstall audits, got %d (%v)", len(expected), len(audits), audits)
	}
	for index, action := range expected {
		if audits[index].Action != action {
			t.Fatalf("audit %d: expected %s, got %s", index, action, audits[index].Action)
		}
	}
}

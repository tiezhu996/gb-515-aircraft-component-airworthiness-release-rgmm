package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/constants"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"gorm.io/gorm"
)

// Sentinel conflict reasons for 装机. Services can errors.Is them while the
// concrete error types carry the blocking record for user-facing messages.
var (
	ErrPartAlreadyInstalled = errors.New("aircraft part already has an active install record")
	ErrInstallSlotOccupied  = errors.New("aircraft serial and install location already occupied by another part")
	ErrPartNotReleased      = errors.New("aircraft part must be released before installation")
	ErrNoActiveInstall      = errors.New("aircraft part has no active install record to close")
)

// InstallBlocker describes the record that rejected an install request so the
// API can tell the caller exactly which row blocked them.
type InstallBlocker struct {
	RecordID       uint   `json:"recordId"`
	PartID         uint   `json:"partId"`
	PartCode       string `json:"partCode"`
	PartName       string `json:"partName"`
	AircraftModel  string `json:"aircraftModel"`
	AircraftSerial string `json:"aircraftSerial"`
	Location       string `json:"location"`
	InstalledBy    string `json:"installedBy"`
}

// PartInstalledConflict means the same part is already mounted somewhere.
type PartInstalledConflict struct{ Blocker InstallBlocker }

func (e *PartInstalledConflict) Error() string { return ErrPartAlreadyInstalled.Error() }
func (e *PartInstalledConflict) Unwrap() error { return ErrPartAlreadyInstalled }

// SlotOccupiedConflict means the (架次, 安装位置) slot already holds another part.
type SlotOccupiedConflict struct{ Blocker InstallBlocker }

func (e *SlotOccupiedConflict) Error() string { return ErrInstallSlotOccupied.Error() }
func (e *SlotOccupiedConflict) Unwrap() error { return ErrInstallSlotOccupied }

// AircraftPartRepository owns all persistence operations for 航空部件.
type AircraftPartRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.AircraftPart], error)
	Get(context.Context, uint) (model.AircraftPart, error)
	Create(context.Context, *model.AircraftPart) error
	Update(context.Context, uint, uint, *model.AircraftPart) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	Install(context.Context, *model.AircraftPart, uint, *model.InstallRecord, string, string) (model.InstallRecord, error)
	Uninstall(context.Context, *model.AircraftPart, uint, *model.InstallRecord, string, string, string) (model.InstallRecord, error)
}

type aircraftPartRepository struct {
	store *Store[model.AircraftPart]
	db    *gorm.DB
}

func NewAircraftPartRepository(db *gorm.DB) AircraftPartRepository {
	return &aircraftPartRepository{store: NewStore[model.AircraftPart](db), db: db}
}

func (r *aircraftPartRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.AircraftPart], error) {
	page, err := r.store.List(ctx, q)
	if err != nil || len(page.Items) == 0 {
		return page, err
	}
	ids := make([]uint, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	var records []model.InstallRecord
	if err := r.db.WithContext(ctx).Where("aircraft_part_id IN ?", ids).
		Order("aircraft_part_id, installed_at DESC, id DESC").Find(&records).Error; err != nil {
		return Page[model.AircraftPart]{}, err
	}
	byPart := make(map[uint][]model.InstallRecord)
	for _, record := range records {
		byPart[record.AircraftPartID] = append(byPart[record.AircraftPartID], record)
	}
	for index := range page.Items {
		page.Items[index].InstallRecords = byPart[page.Items[index].ID]
	}
	return page, nil
}

func (r *aircraftPartRepository) Get(ctx context.Context, id uint) (model.AircraftPart, error) {
	var item model.AircraftPart
	err := r.db.WithContext(ctx).Preload("InstallRecords", func(db *gorm.DB) *gorm.DB {
		return db.Order("installed_at DESC, id DESC")
	}).First(&item, id).Error
	return item, err
}

func (r *aircraftPartRepository) Create(ctx context.Context, item *model.AircraftPart) error {
	return r.store.Create(ctx, item)
}
func (r *aircraftPartRepository) Update(ctx context.Context, id, version uint, item *model.AircraftPart) error {
	item.InstallRecords = nil
	return r.store.Update(ctx, id, version, item)
}
func (r *aircraftPartRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *aircraftPartRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

// InstallSlotKey is the normalized unique identity of one mount slot on one
// aircraft 架次. The NUL byte separator cannot appear in trimmed input.
func InstallSlotKey(aircraftSerial, location string) string {
	return strings.ToLower(strings.TrimSpace(aircraftSerial)) + "\x00" + strings.ToLower(strings.TrimSpace(location))
}

func (r *aircraftPartRepository) Install(ctx context.Context, part *model.AircraftPart, expectedVersion uint, record *model.InstallRecord, actor, requestID string) (model.InstallRecord, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var activeByPart model.InstallRecord
		if err := tx.Where("aircraft_part_id = ? AND removed_at IS NULL", part.ID).First(&activeByPart).Error; err == nil {
			blocker, buildErr := r.buildBlocker(tx, activeByPart)
			if buildErr != nil {
				return buildErr
			}
			return &PartInstalledConflict{Blocker: blocker}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var activeBySlot model.InstallRecord
		if err := tx.Where("slot_key = ? AND removed_at IS NULL", record.SlotKey).First(&activeBySlot).Error; err == nil {
			blocker, buildErr := r.buildBlocker(tx, activeBySlot)
			if buildErr != nil {
				return buildErr
			}
			return &SlotOccupiedConflict{Blocker: blocker}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Atomic status guard: only a released part at the expected version may
		// become installed, so concurrent releases/holds cannot race in.
		result := tx.Model(&model.AircraftPart{}).
			Where("id = ? AND version = ? AND status = ?", part.ID, expectedVersion, string(constants.PartStateReleased)).
			Updates(map[string]any{"status": string(constants.PartStateInstalled), "version": gorm.Expr("version + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return r.installStateConflict(tx, part.ID)
		}

		record.AircraftPartID = part.ID
		record.InstalledAt = now
		record.InstallRequest = requestID
		if record.InstalledBy == "" {
			record.InstalledBy = actor
		}
		record.CreatedAt = now
		record.UpdatedAt = now
		if err := tx.Create(record).Error; err != nil {
			if isDuplicateKey(err) {
				// Concurrent install slipped past the pre-checks: surface whichever
				// open record now holds the unique key.
				return r.duplicateConflict(tx, part.ID, record.SlotKey)
			}
			return err
		}

		part.Status = string(constants.PartStateInstalled)
		part.Version = expectedVersion + 1
		part.UpdatedAt = now
		return appendAudit(tx, actor, requestID, "install", "AircraftPart", part.ID, "released", "installed",
			"installed on "+record.AircraftModel+" "+record.AircraftSerial+" at "+record.Location+" by "+record.InstalledBy+
				" (install record #"+strconv.FormatUint(uint64(record.ID), 10)+")")
	})
	if err != nil {
		return model.InstallRecord{}, err
	}
	return *record, nil
}

func (r *aircraftPartRepository) Uninstall(ctx context.Context, part *model.AircraftPart, expectedVersion uint, record *model.InstallRecord, actor, requestID, reason string) (model.InstallRecord, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active model.InstallRecord
		if err := tx.Where("aircraft_part_id = ? AND removed_at IS NULL", part.ID).First(&active).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoActiveInstall
			}
			return err
		}

		result := tx.Model(&model.AircraftPart{}).
			Where("id = ? AND version = ? AND status = ?", part.ID, expectedVersion, string(constants.PartStateInstalled)).
			Updates(map[string]any{"status": string(constants.PartStateInspection), "version": gorm.Expr("version + 1"), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return r.uninstallStateConflict(tx, part.ID)
		}

		closeResult := tx.Model(&model.InstallRecord{}).Where("id = ? AND removed_at IS NULL", active.ID).
			Updates(map[string]any{
				"removed_by":        actor,
				"removed_at":        now,
				"remove_reason":     reason,
				"remove_request_id": requestID,
				"part_active":       active.ID,
				"slot_active":       active.ID,
				"updated_at":        now,
			})
		if closeResult.Error != nil {
			return closeResult.Error
		}
		if closeResult.RowsAffected == 0 {
			// A concurrent uninstall closed the record first.
			return ErrNoActiveInstall
		}

		active.RemovedBy = actor
		active.RemovedAt = &now
		active.RemoveReason = reason
		active.RemoveRequestID = requestID
		active.PartActive = active.ID
		active.SlotActive = active.ID
		active.UpdatedAt = now
		*record = active

		part.Status = string(constants.PartStateInspection)
		part.Version = expectedVersion + 1
		part.UpdatedAt = now
		return appendAudit(tx, actor, requestID, "uninstall", "AircraftPart", part.ID, "installed", "inspection",
			"uninstalled from "+active.AircraftModel+" "+active.AircraftSerial+" at "+active.Location+
				" (install record #"+strconv.FormatUint(uint64(active.ID), 10)+"); reason: "+reason)
	})
	if err != nil {
		return model.InstallRecord{}, err
	}
	return *record, nil
}

func (r *aircraftPartRepository) buildBlocker(tx *gorm.DB, record model.InstallRecord) (InstallBlocker, error) {
	blocker := InstallBlocker{
		RecordID: record.ID, PartID: record.AircraftPartID,
		AircraftModel: record.AircraftModel, AircraftSerial: record.AircraftSerial,
		Location: record.Location, InstalledBy: record.InstalledBy,
	}
	var part model.AircraftPart
	if err := tx.Select("code", "name").First(&part, record.AircraftPartID).Error; err != nil {
		return InstallBlocker{}, err
	}
	blocker.PartCode = part.Code
	blocker.PartName = part.Name
	return blocker, nil
}

func (r *aircraftPartRepository) duplicateConflict(tx *gorm.DB, partID uint, slotKey string) error {
	var active model.InstallRecord
	if err := tx.Where("aircraft_part_id = ? AND removed_at IS NULL", partID).First(&active).Error; err == nil {
		blocker, buildErr := r.buildBlocker(tx, active)
		if buildErr != nil {
			return buildErr
		}
		return &PartInstalledConflict{Blocker: blocker}
	}
	if err := tx.Where("slot_key = ? AND removed_at IS NULL", slotKey).First(&active).Error; err == nil {
		blocker, buildErr := r.buildBlocker(tx, active)
		if buildErr != nil {
			return buildErr
		}
		return &SlotOccupiedConflict{Blocker: blocker}
	}
	return ErrInstallSlotOccupied
}

// installStateConflict distinguishes a stale optimistic-lock version from a
// part whose status no longer allows install when the guarded update matched
// no row.
func (r *aircraftPartRepository) installStateConflict(tx *gorm.DB, partID uint) error {
	var part model.AircraftPart
	if err := tx.Select("id", "status", "version").First(&part, partID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	if part.Status != string(constants.PartStateReleased) {
		return ErrPartNotReleased
	}
	return ErrVersionConflict
}

// uninstallStateConflict distinguishes a stale version from a part whose open
// install record already disappeared when the guarded update matched no row.
func (r *aircraftPartRepository) uninstallStateConflict(tx *gorm.DB, partID uint) error {
	var part model.AircraftPart
	if err := tx.Select("id", "status", "version").First(&part, partID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	if part.Status != string(constants.PartStateInstalled) {
		return ErrNoActiveInstall
	}
	return ErrVersionConflict
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate") || strings.Contains(text, "unique constraint")
}

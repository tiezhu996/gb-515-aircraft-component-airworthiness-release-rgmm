package repository

import (
	"context"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/model"
	"gorm.io/gorm"
)

// AircraftPartRepository owns all persistence operations for 航空部件.
type AircraftPartRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.AircraftPart], error)
	Get(context.Context, uint) (model.AircraftPart, error)
	Create(context.Context, *model.AircraftPart) error
	Update(context.Context, uint, uint, *model.AircraftPart) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	ActiveInstallationForPart(context.Context, uint) (model.PartInstallation, error)
	ActiveInstallationForSlot(context.Context, string, string) (model.PartInstallation, error)
	ListInstallations(context.Context, uint) ([]model.PartInstallation, error)
	Install(context.Context, uint, uint, *model.AircraftPart, *model.PartInstallation, string, string, string) error
	Uninstall(context.Context, uint, uint, *model.AircraftPart, *model.PartInstallation, string, string, string) error
}

type aircraftPartRepository struct {
	store *Store[model.AircraftPart]
	db    *gorm.DB
}

func NewAircraftPartRepository(db *gorm.DB) AircraftPartRepository {
	return &aircraftPartRepository{store: NewStore[model.AircraftPart](db), db: db}
}

func (r *aircraftPartRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.AircraftPart], error) {
	return r.store.List(ctx, q)
}
func (r *aircraftPartRepository) Get(ctx context.Context, id uint) (model.AircraftPart, error) {
	var item model.AircraftPart
	err := r.db.WithContext(ctx).Preload("Installations", func(db *gorm.DB) *gorm.DB {
		return db.Order("id DESC")
	}).First(&item, id).Error
	return item, err
}
func (r *aircraftPartRepository) Create(ctx context.Context, item *model.AircraftPart) error {
	return r.store.Create(ctx, item)
}
func (r *aircraftPartRepository) Update(ctx context.Context, id, version uint, item *model.AircraftPart) error {
	item.Installations = nil
	return r.store.Update(ctx, id, version, item)
}
func (r *aircraftPartRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *aircraftPartRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

func (r *aircraftPartRepository) ActiveInstallationForPart(ctx context.Context, partID uint) (model.PartInstallation, error) {
	var item model.PartInstallation
	err := r.db.WithContext(ctx).Where("part_id = ? AND active = ?", partID, true).First(&item).Error
	return item, err
}

func (r *aircraftPartRepository) ActiveInstallationForSlot(ctx context.Context, aircraftTail, position string) (model.PartInstallation, error) {
	var item model.PartInstallation
	err := r.db.WithContext(ctx).Where("aircraft_tail = ? AND position = ? AND active = ?", aircraftTail, position, true).First(&item).Error
	return item, err
}

func (r *aircraftPartRepository) ListInstallations(ctx context.Context, partID uint) ([]model.PartInstallation, error) {
	items := make([]model.PartInstallation, 0)
	err := r.db.WithContext(ctx).Where("part_id = ?", partID).Order("id DESC").Find(&items).Error
	return items, err
}

// Install writes the 装机履历, flips the part to installed and appends the
// audit entry in one transaction so a partial install can never be observed.
func (r *aircraftPartRepository) Install(ctx context.Context, id, expectedVersion uint, part *model.AircraftPart, installation *model.PartInstallation, actor, requestID, detail string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(installation).Error; err != nil {
			return err
		}
		part.Installations = nil
		if err := optimisticUpdate(tx, id, expectedVersion, part); err != nil {
			return err
		}
		return appendAudit(tx, actor, requestID, "install", "AircraftPart", id, "released", part.Status, detail)
	})
}

// Uninstall closes the active installation record instead of deleting it, so
// the 装机履历 stays readable on the part detail forever.
func (r *aircraftPartRepository) Uninstall(ctx context.Context, id, expectedVersion uint, part *model.AircraftPart, installation *model.PartInstallation, actor, requestID, detail string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.PartInstallation{}).
			Where("id = ? AND active = ?", installation.ID, true).
			Updates(map[string]any{
				"active":          false,
				"removed_at":      installation.RemovedAt,
				"removed_by":      installation.RemovedBy,
				"removal_reason":  installation.RemovalReason,
				"active_part_key": nil,
				"active_slot_key": nil,
				"updated_at":      installation.RemovedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		part.Installations = nil
		if err := optimisticUpdate(tx, id, expectedVersion, part); err != nil {
			return err
		}
		return appendAudit(tx, actor, requestID, "uninstall", "AircraftPart", id, "installed", part.Status, detail)
	})
}

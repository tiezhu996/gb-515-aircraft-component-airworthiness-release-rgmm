package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/service"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, service.ErrForbidden):
		util.Fail(c, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, service.ErrLocked), errors.Is(err, service.ErrSeparationOfDuty):
		util.Fail(c, http.StatusConflict, "control_conflict", err.Error())
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}

// handleInstallError maps 装机/卸载 failures. Rejected installs return 409 with
// the exact blocking 装机履历 record so operators can see who/what holds the
// part or the mount slot.
func handleInstallError(c *gin.Context, err error) {
	var partConflict *repository.PartInstalledConflict
	if errors.As(err, &partConflict) {
		b := partConflict.Blocker
		message := fmt.Sprintf(
			"装机被拒：部件 %s（%s）已有在装记录（履历 #%d），%s 装机于 %s %s 的 %s，尚未卸载，不可重复装机",
			b.PartCode, b.PartName, b.RecordID, b.InstalledBy, b.AircraftModel, b.AircraftSerial, b.Location,
		)
		util.FailWithDetails(c, http.StatusConflict, "install_part_active", message, gin.H{"reason": "part_already_installed", "blocker": b})
		return
	}
	var slotConflict *repository.SlotOccupiedConflict
	if errors.As(err, &slotConflict) {
		b := slotConflict.Blocker
		message := fmt.Sprintf(
			"装机被拒：架次 %s 的安装位置 %s 已被部件 %s（%s）占用（履历 #%d，装机人 %s），同一位置不可挂两件部件",
			b.AircraftSerial, b.Location, b.PartCode, b.PartName, b.RecordID, b.InstalledBy,
		)
		util.FailWithDetails(c, http.StatusConflict, "install_slot_occupied", message, gin.H{"reason": "slot_occupied", "blocker": b})
		return
	}
	switch {
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, repository.ErrNoActiveInstall):
		util.Fail(c, http.StatusConflict, "no_active_install", "部件当前没有在装记录，无法卸载")
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}

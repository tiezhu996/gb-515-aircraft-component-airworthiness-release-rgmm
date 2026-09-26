package model

import "time"

// AircraftPart models 航空部件 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type AircraftPart struct {
	BaseModel
	Facility       string          `json:"facility" gorm:"size:120;index"`
	Owner          string          `json:"owner" gorm:"size:120;index"`
	Category       string          `json:"category" gorm:"size:80;index"`
	RiskLevel      string          `json:"riskLevel" gorm:"size:32;index"`
	MetricValue    float64         `json:"metricValue"`
	MetricUnit     string          `json:"metricUnit" gorm:"size:24"`
	EffectiveAt    time.Time       `json:"effectiveAt"`
	Evidence       string          `json:"evidence" gorm:"size:2000"`
	RelatedCode    string          `json:"relatedCode" gorm:"size:64;index"`
	InstallRecords []InstallRecord `json:"installRecords,omitempty" gorm:"foreignKey:AircraftPartID"`
}

func (item *AircraftPart) GetBase() *BaseModel { return &item.BaseModel }

func (item AircraftPart) TableName() string { return "aircraft_parts" }

var AircraftPartInitialStatus = "received"

// InstallRecord is an append-only entry of the 装机履历 of an 航空部件. Each
// install creates one row; uninstall closes the same row. Removed rows are
// retained permanently and preloaded on part detail, so 装到哪架机上 always
// stays traceable.
type InstallRecord struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	AircraftPartID  uint       `json:"aircraftPartId" gorm:"not null;index;uniqueIndex:idx_install_part_active,priority:1"`
	AircraftModel   string     `json:"aircraftModel" gorm:"size:120;not null"`
	AircraftSerial  string     `json:"aircraftSerial" gorm:"size:80;not null"`
	Location        string     `json:"location" gorm:"size:120;not null"`
	InstalledBy     string     `json:"installedBy" gorm:"size:80;not null;index"`
	InstalledAt     time.Time  `json:"installedAt" gorm:"not null;index"`
	InstallRequest  string     `json:"installRequestId" gorm:"size:64;not null;index"`
	RemovedBy       string     `json:"removedBy" gorm:"size:80;index"`
	RemovedAt       *time.Time `json:"removedAt" gorm:"index"`
	RemoveReason    string     `json:"removeReason" gorm:"size:1000"`
	RemoveRequestID string     `json:"removeRequestId" gorm:"size:64;index"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	// PartActive is 0 while the record is open (part still installed) and set
	// to its own ID on removal. Together with the slot-key columns it gives
	// database-enforced, dialect-portable uniqueness for the open record only.
	PartActive uint   `json:"-" gorm:"not null;default:0;uniqueIndex:idx_install_part_active,priority:2"`
	SlotKey    string `json:"-" gorm:"size:300;not null;default:'';uniqueIndex:idx_install_slot_active,priority:1"`
	SlotActive uint   `json:"-" gorm:"not null;default:0;uniqueIndex:idx_install_slot_active,priority:2"`
}

func (item InstallRecord) TableName() string { return "install_records" }

// Active reports whether the part is still mounted under this record.
func (item InstallRecord) Active() bool { return item.RemovedAt == nil }

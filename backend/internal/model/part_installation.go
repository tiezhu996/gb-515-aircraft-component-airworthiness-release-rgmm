package model

import "time"

// PartInstallation is the append-only 装机履历 of an AircraftPart. Records are
// never deleted: uninstall only closes the active record so the full history
// stays visible on the part detail. ActivePartKey and ActiveSlotKey are
// nullable unique sentinels that let the database itself guarantee a part is
// installed at most once and a 架次/安装位置 slot hosts at most one part.
type PartInstallation struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	PartID        uint       `json:"partId" gorm:"not null;index"`
	PartCode      string     `json:"partCode" gorm:"size:64;not null;index"`
	AircraftModel string     `json:"aircraftModel" gorm:"size:80;not null"`
	AircraftTail  string     `json:"aircraftTail" gorm:"size:80;not null;index"`
	Position      string     `json:"position" gorm:"size:120;not null"`
	Installer     string     `json:"installer" gorm:"size:120;not null"`
	InstalledBy   string     `json:"installedBy" gorm:"size:80;not null"`
	InstalledAt   time.Time  `json:"installedAt" gorm:"not null"`
	Active        bool       `json:"active" gorm:"not null;default:true;index"`
	RemovedAt     *time.Time `json:"removedAt"`
	RemovedBy     string     `json:"removedBy" gorm:"size:80"`
	RemovalReason string     `json:"removalReason" gorm:"size:500"`
	ActivePartKey *string    `json:"-" gorm:"size:80;uniqueIndex"`
	ActiveSlotKey *string    `json:"-" gorm:"size:240;uniqueIndex"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (item PartInstallation) TableName() string { return "part_installations" }

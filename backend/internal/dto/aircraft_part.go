package dto

import "time"

// CreateAircraftPart is the public write contract for 航空部件. Status is deliberately
// omitted so callers cannot bypass the service state machine.
type CreateAircraftPart struct {
	Code        string    `json:"code" binding:"required,min=2,max=64"`
	Name        string    `json:"name" binding:"required,min=2,max=160"`
	Description string    `json:"description" binding:"max=1000"`
	Facility    string    `json:"facility" binding:"required,max=120"`
	Owner       string    `json:"owner" binding:"required,max=120"`
	Category    string    `json:"category" binding:"required,max=80"`
	RiskLevel   string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt time.Time `json:"effectiveAt" binding:"required"`
	Evidence    string    `json:"evidence" binding:"max=2000"`
	RelatedCode string    `json:"relatedCode" binding:"max=64"`
}

type UpdateAircraftPart struct {
	ExpectedVersion uint      `json:"expectedVersion" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=160"`
	Description     string    `json:"description" binding:"max=1000"`
	Facility        string    `json:"facility" binding:"required,max=120"`
	Owner           string    `json:"owner" binding:"required,max=120"`
	Category        string    `json:"category" binding:"required,max=80"`
	RiskLevel       string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue     float64   `json:"metricValue"`
	MetricUnit      string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt     time.Time `json:"effectiveAt" binding:"required"`
	Evidence        string    `json:"evidence" binding:"max=2000"`
	RelatedCode     string    `json:"relatedCode" binding:"max=64"`
}

// InstallAircraftPart registers the 装机履历 required to move a released part
// into installed: 机型、架次、安装位置和装机人 are all mandatory.
type InstallAircraftPart struct {
	ExpectedVersion uint   `json:"expectedVersion" binding:"required"`
	AircraftModel   string `json:"aircraftModel" binding:"required,min=2,max=80"`
	AircraftTail    string `json:"aircraftTail" binding:"required,min=2,max=80"`
	Position        string `json:"position" binding:"required,min=2,max=120"`
	Installer       string `json:"installer" binding:"required,min=2,max=120"`
}

// UninstallAircraftPart closes the active installation; the reason is kept on
// the historical record forever.
type UninstallAircraftPart struct {
	ExpectedVersion uint   `json:"expectedVersion" binding:"required"`
	Reason          string `json:"reason" binding:"required,min=3,max=500"`
}

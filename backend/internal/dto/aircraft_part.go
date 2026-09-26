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

// InstallAircraftPart registers one entry of 装机履历 and moves a released part
// to installed. InstalledBy may be blank to default to the authenticated actor.
type InstallAircraftPart struct {
	ExpectedVersion uint   `json:"expectedVersion" binding:"required"`
	AircraftModel   string `json:"aircraftModel" binding:"required,min=1,max=120"`
	AircraftSerial  string `json:"aircraftSerial" binding:"required,min=1,max=80"`
	Location        string `json:"location" binding:"required,min=1,max=120"`
	InstalledBy     string `json:"installedBy" binding:"max=80"`
}

// UninstallAircraftPart closes the open 装机履历 entry and returns the part to
// inspection. A reason is mandatory so every removal stays explainable.
type UninstallAircraftPart struct {
	ExpectedVersion uint   `json:"expectedVersion" binding:"required"`
	Reason          string `json:"reason" binding:"required,min=1,max=1000"`
}

package constants

// Shared status values are mirrored in frontend/src/types/status.ts. Keeping
// the lists explicit makes state-machine drift visible during code review.

type PartState string

const (
	PartStateReceived   PartState = "received"
	PartStateInspection PartState = "inspection"
	PartStateHold       PartState = "hold"
	PartStateReleased   PartState = "released"
	PartStateInstalled  PartState = "installed"
	PartStateRetired    PartState = "retired"
)

var AllPartState = []string{"received", "inspection", "hold", "released", "installed", "retired"}

type AuthorizationState string

const (
	AuthorizationStateDraft      AuthorizationState = "draft"
	AuthorizationStateReview     AuthorizationState = "review"
	AuthorizationStateApproved   AuthorizationState = "approved"
	AuthorizationStateRestricted AuthorizationState = "restricted"
	AuthorizationStateRevoked    AuthorizationState = "revoked"
)

var AllAuthorizationState = []string{"draft", "review", "approved", "restricted", "revoked"}

// AircraftPartTransitions deliberately excludes "installed" as a generic
// target: released -> installed only happens through the dedicated install
// endpoint that registers 机型/架次/安装位置/装机人, and installed parts leave
// the state only through the uninstall endpoint back to inspection.
var AircraftPartTransitions = map[string]map[string]bool{
	"received":   {"inspection": true, "hold": true},
	"inspection": {"hold": true, "released": true, "received": true},
	"hold":       {"released": true, "retired": true, "inspection": true},
	"released":   {"retired": true, "hold": true},
	"installed":  {},
	"retired":    {"released": true},
}

var InspectionTaskTransitions = map[string]map[string]bool{
	"planned": {"running": true},
	"running": {"passed": true, "failed": true},
	"passed":  {},
	"failed":  {"running": true},
}

var CertificateRecordTransitions = map[string]map[string]bool{
	"draft":   {"valid": true},
	"valid":   {"expired": true, "revoked": true},
	"expired": {"revoked": true},
	"revoked": {},
}

var ReleaseAuthorizationTransitions = map[string]map[string]bool{
	"draft":      {"review": true},
	"review":     {"approved": true, "restricted": true, "draft": true},
	"approved":   {"restricted": true, "revoked": true},
	"restricted": {"revoked": true},
	"revoked":    {},
}

func CanTransition(graph map[string]map[string]bool, from, to string) bool {
	targets, exists := graph[from]
	return exists && targets[to]
}

package constants

import "testing"

func TestAircraftPartTransitionGraph(t *testing.T) {
	if !CanTransition(AircraftPartTransitions, "received", "inspection") {
		t.Fatalf("expected received -> inspection transition to be allowed")
	}
	if CanTransition(AircraftPartTransitions, "received", "unknown") {
		t.Fatal("unknown status must never be accepted")
	}
}

func TestAircraftPartInstalledStateIsEndpointDriven(t *testing.T) {
	if CanTransition(AircraftPartTransitions, "released", "installed") {
		t.Fatal("installation must register 装机履历 through the dedicated install endpoint")
	}
	for _, target := range []string{"received", "inspection", "hold", "released", "retired"} {
		if CanTransition(AircraftPartTransitions, "installed", target) {
			t.Fatalf("installed parts must not transition to %s directly", target)
		}
	}
}

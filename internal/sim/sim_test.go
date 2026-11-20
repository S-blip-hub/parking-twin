package sim

import (
	"path/filepath"
	"testing"

	"github.com/S-blip-hub/parking-twin/internal/campus"
)

func base() Scenario {
	s := Scenario{Name: "test", VisitorCars: 300}
	s.Defaults()
	return s
}

func TestCampusHasThirtyAnchorsAndSixtyZones(t *testing.T) {
	c := campus.New()
	if len(c.Anchors) != 30 {
		t.Fatalf("anchors = %d, want 30", len(c.Anchors))
	}
	if len(c.Zones) != 60 {
		t.Fatalf("zones = %d, want 60", len(c.Zones))
	}
	for _, a := range c.Anchors {
		if c.Zones[a.HomeZone].AnchorID != a.ID {
			t.Errorf("%s: home zone not linked back", a.Name)
		}
	}
}

func TestBaysAreNeverOverfilled(t *testing.T) {
	res := Run(campus.New(), Scenario{Name: "busy", VisitorCars: 2000, CarOwnership: 1.6})
	for step, snap := range res.Occupancy {
		for i, z := range res.Campus.Zones {
			if snap[i].Cars > z.CarCap || snap[i].TwoW > z.TwoWCap {
				t.Fatalf("step %s zone %s over capacity: %+v", Clock(step), z.Name, snap[i])
			}
			if snap[i].Cars < 0 || snap[i].OverflowCars < 0 {
				t.Fatalf("negative count at %s in %s", Clock(step), z.Name)
			}
		}
	}
}

func TestEveryResidentVehicleIsHomeAtMidnight(t *testing.T) {
	res := Run(campus.New(), base())
	first := res.Occupancy[0]
	total := [2]int{}
	for i := range res.Campus.Zones {
		total[campus.Car] += first[i].Cars + first[i].OverflowCars
		total[campus.TwoWheeler] += first[i].TwoW + first[i].OverflowTwoW
	}
	if total != res.Vehicles {
		t.Fatalf("vehicles parked at 00:00 = %v, fleet = %v", total, res.Vehicles)
	}
}

func TestPlentyOfBaysMeansNoUnmetDemand(t *testing.T) {
	s := base()
	s.ExtraCarBays = map[string]int{}
	for _, z := range campus.New().Zones {
		s.ExtraCarBays[z.Name] = 1000
	}
	s.MaxWalkCarM = 5000
	res := Run(campus.New(), s)
	if res.Unmet[campus.Car] != 0 {
		t.Fatalf("unmet car arrivals = %d, want 0", res.Unmet[campus.Car])
	}
}

func TestControlsMoveResultsTheRightWay(t *testing.T) {
	c := campus.New()
	growth := Scenario{Name: "growth", VisitorCars: 380, CarOwnership: 1.4}
	priced := growth
	priced.CoreCarShift = 0.3
	event := base()
	event.Events = []Event{{Name: "fest", Cluster: "auditorium", Cars: 900, ArriveFrom: 8, ArriveTo: 10, DwellH: 5}}

	g, p, b, e := Run(c, growth), Run(c, priced), Run(c, base()), Run(c, event)
	if !(g.UnmetShare(campus.Car) > b.UnmetShare(campus.Car)) {
		t.Errorf("growth should raise unmet demand: %.3f vs %.3f", g.UnmetShare(campus.Car), b.UnmetShare(campus.Car))
	}
	if !(p.UnmetShare(campus.Car) < g.UnmetShare(campus.Car)) {
		t.Errorf("core pricing should cut unmet demand: %.3f vs %.3f", p.UnmetShare(campus.Car), g.UnmetShare(campus.Car))
	}
	if !(e.Unmet[campus.Car] > b.Unmet[campus.Car]) {
		t.Errorf("an event should add unmet arrivals")
	}
}

func TestClosedZonesStayEmpty(t *testing.T) {
	s := base()
	s.ClosedZones = []string{"Academic P1"}
	res := Run(campus.New(), s)
	for i, z := range res.Campus.Zones {
		if z.Name != "Academic P1" {
			continue
		}
		for _, snap := range res.Occupancy {
			if snap[i].Cars != 0 || snap[i].TwoW != 0 {
				t.Fatal("closed zone has parked vehicles")
			}
		}
	}
}

func TestRunsAreReproducible(t *testing.T) {
	a, b := Run(campus.New(), base()), Run(campus.New(), base())
	if a.Unmet != b.Unmet || a.Arrivals != b.Arrivals || a.WalkSum != b.WalkSum {
		t.Fatal("same seed gave different results")
	}
}

func TestScenarioFileLoads(t *testing.T) {
	list, err := LoadScenarios(filepath.Join("..", "..", "scenarios", "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 5 || list[0].Name != "baseline" {
		t.Fatalf("unexpected scenarios: %d", len(list))
	}
}

func TestValidateRejectsBadWindows(t *testing.T) {
	s := base()
	s.Events = []Event{{Name: "bad", Cluster: "auditorium", ArriveFrom: 10, ArriveTo: 8, DwellH: 1}}
	if s.Validate() == nil {
		t.Fatal("expected an error for an event that ends before it starts")
	}
}

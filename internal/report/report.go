// Package report writes simulation results as CSV and JSON for analysis.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/S-blip-hub/parking-twin/internal/campus"
	"github.com/S-blip-hub/parking-twin/internal/sim"
)

// Summary is the headline result of one scenario.
type Summary struct {
	Scenario         string            `json:"scenario"`
	Description      string            `json:"description"`
	Cars             int               `json:"resident_cars"`
	TwoWheelers      int               `json:"resident_two_wheelers"`
	CarArrivals      int               `json:"car_arrivals"`
	TwoWArrivals     int               `json:"two_wheeler_arrivals"`
	UnmetCarPct      float64           `json:"unmet_car_arrivals_pct"`
	UnmetTwoWPct     float64           `json:"unmet_two_wheeler_arrivals_pct"`
	MeanWalkCarM     float64           `json:"mean_walk_car_m"`
	MeanWalkTwoWM    float64           `json:"mean_walk_two_wheeler_m"`
	PeakCarsParked   int               `json:"peak_cars_parked_at_destinations"`
	PeakTime         string            `json:"peak_time"`
	ZonesOver85AtPk  int               `json:"zones_over_85pct_at_peak"`
	ResidentialOverf int               `json:"max_residential_overflow_cars"`
	Clusters         []sim.ClusterPeak `json:"clusters"`
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// Summarise reduces a run to its headline numbers.
func Summarise(res *sim.Result) Summary {
	s := Summary{
		Scenario: res.Scenario.Name, Description: res.Scenario.Description,
		Cars: res.Vehicles[campus.Car], TwoWheelers: res.Vehicles[campus.TwoWheeler],
		CarArrivals: res.Arrivals[campus.Car], TwoWArrivals: res.Arrivals[campus.TwoWheeler],
		UnmetCarPct:   round1(100 * res.UnmetShare(campus.Car)),
		UnmetTwoWPct:  round1(100 * res.UnmetShare(campus.TwoWheeler)),
		MeanWalkCarM:  round1(res.MeanWalk(campus.Car)),
		MeanWalkTwoWM: round1(res.MeanWalk(campus.TwoWheeler)),
		Clusters:      res.Peaks(),
	}
	zones := res.Campus.Zones
	peakStep := 0
	for step, snap := range res.Occupancy {
		parked, resOver := 0, 0
		for i, z := range zones {
			if z.Cluster == campus.Home {
				resOver += snap[i].OverflowCars
				continue
			}
			parked += snap[i].Cars
		}
		if parked > s.PeakCarsParked {
			s.PeakCarsParked, peakStep = parked, step
		}
		if resOver > s.ResidentialOverf {
			s.ResidentialOverf = resOver
		}
	}
	s.PeakTime = sim.Clock(peakStep)
	for i, z := range zones {
		if z.Cluster != campus.Home && z.CarCap > 0 && float64(res.Occupancy[peakStep][i].Cars) >= 0.85*float64(z.CarCap) {
			s.ZonesOver85AtPk++
		}
	}
	return s
}

// WriteRun writes occupancy.csv and summary.json for one scenario into dir.
func WriteRun(dir string, res *sim.Result) (Summary, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Summary{}, err
	}
	f, err := os.Create(filepath.Join(dir, "occupancy.csv"))
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"time", "zone", "cluster", "cars", "car_bays", "two_wheelers", "two_wheeler_bays",
		"overflow_cars", "overflow_two_wheelers"})
	for step, snap := range res.Occupancy {
		if step%3 != 0 { // 15-minute resolution is enough for analysis
			continue
		}
		for i, z := range res.Campus.Zones {
			o := snap[i]
			_ = w.Write([]string{sim.Clock(step), z.Name, string(z.Cluster), itoa(o.Cars), itoa(z.CarCap),
				itoa(o.TwoW), itoa(z.TwoWCap), itoa(o.OverflowCars), itoa(o.OverflowTwoW)})
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return Summary{}, err
	}
	s := Summarise(res)
	raw, _ := json.MarshalIndent(s, "", "  ")
	return s, os.WriteFile(filepath.Join(dir, "summary.json"), append(raw, '\n'), 0o644)
}

// WriteComparison writes one row per scenario.
func WriteComparison(path string, list []Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"scenario", "resident_cars", "car_arrivals", "unmet_car_pct", "unmet_two_wheeler_pct",
		"mean_walk_car_m", "peak_cars_parked", "peak_time", "zones_over_85pct", "max_residential_overflow_cars"})
	for _, s := range list {
		_ = w.Write([]string{s.Scenario, itoa(s.Cars), itoa(s.CarArrivals), ftoa(s.UnmetCarPct), ftoa(s.UnmetTwoWPct),
			ftoa(s.MeanWalkCarM), itoa(s.PeakCarsParked), s.PeakTime, itoa(s.ZonesOver85AtPk), itoa(s.ResidentialOverf)})
	}
	w.Flush()
	return w.Error()
}

// WriteLayout writes the anchors and zones so maps can be drawn.
func WriteLayout(dir string, c *campus.Campus) error {
	zf, err := os.Create(filepath.Join(dir, "zones.csv"))
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := csv.NewWriter(zf)
	_ = zw.Write([]string{"id", "name", "cluster", "x", "y", "car_bays", "two_wheeler_bays"})
	for _, z := range c.Zones {
		_ = zw.Write([]string{itoa(z.ID), z.Name, string(z.Cluster), ftoa(z.X), ftoa(z.Y), itoa(z.CarCap), itoa(z.TwoWCap)})
	}
	zw.Flush()

	af, err := os.Create(filepath.Join(dir, "anchors.csv"))
	if err != nil {
		return err
	}
	defer af.Close()
	aw := csv.NewWriter(af)
	_ = aw.Write([]string{"id", "name", "kind", "x", "y", "residents", "cars", "two_wheelers"})
	for _, a := range c.Anchors {
		cars := int(float64(a.Residents)*a.CarRate + 0.5)
		twoW := int(float64(a.Residents)*a.TwoWRate + 0.5)
		_ = aw.Write([]string{itoa(a.ID), a.Name, string(a.Kind), ftoa(a.X), ftoa(a.Y), itoa(a.Residents), itoa(cars), itoa(twoW)})
	}
	aw.Flush()
	if zw.Error() != nil {
		return zw.Error()
	}
	return aw.Error()
}

func itoa(n int) string     { return strconv.Itoa(n) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Print writes a short table to stdout.
func Print(list []Summary) {
	fmt.Printf("%-16s %8s %10s %10s %10s %8s %6s\n", "scenario", "cars", "unmet car", "unmet 2W", "walk car", "peak", "≥85%")
	for _, s := range list {
		fmt.Printf("%-16s %8d %9.1f%% %9.1f%% %8.0f m %8s %6d\n", s.Scenario, s.Cars, s.UnmetCarPct, s.UnmetTwoWPct,
			s.MeanWalkCarM, s.PeakTime, s.ZonesOver85AtPk)
	}
}

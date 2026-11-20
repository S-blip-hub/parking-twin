// Package sim runs a one-day, 5-minute-step simulation of parking demand.
//
// Every vehicle starts the day in the zone attached to its accommodation
// anchor. Vehicles make outings (work, study, market, sports, visits) drawn
// from purpose-specific time profiles. On arrival a vehicle takes the nearest
// zone with a free bay of its type within the walking limit; if none exists the
// arrival is unmet and the vehicle is counted as overflow (on-street or
// informal parking) at the nearest zone.
package sim

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/S-blip-hub/parking-twin/internal/campus"
)

const (
	StepMin = 5
	Steps   = 24 * 60 / StepMin
)

// StepOf converts a clock time in hours to a step index, clamped to the day.
func StepOf(h float64) int {
	s := int(math.Round(h * 60 / StepMin))
	if s < 0 {
		return 0
	}
	if s >= Steps {
		return Steps - 1
	}
	return s
}

type outing struct {
	cluster campus.Cluster
	arrive  int
	leave   int
}

type vehicle struct {
	kind    campus.Vehicle
	home    int // home zone, -1 for visitors
	outings []outing
}

type arrival struct {
	v       int
	cluster campus.Cluster
	leave   int
}

type departure struct {
	zone     int
	kind     campus.Vehicle
	overflow bool
}

// ZoneStep is the state of one zone at one step.
type ZoneStep struct {
	Cars, TwoW                 int
	OverflowCars, OverflowTwoW int
}

// Result holds everything a run produces.
type Result struct {
	Scenario    Scenario
	Campus      *campus.Campus
	Occupancy   [][]ZoneStep // [step][zone]
	Arrivals    [2]int       // by vehicle type, destination arrivals only
	Unmet       [2]int
	WalkSum     [2]float64
	UnmetByStep []int
	Vehicles    [2]int
}

type runner struct {
	c        *campus.Campus
	s        Scenario
	rng      *rand.Rand
	zones    []campus.Zone
	byClust  map[campus.Cluster][]int
	used     [][2]int
	overflow [][2]int
	res      *Result
}

// Run simulates one weekday for the scenario.
func Run(c *campus.Campus, s Scenario) *Result {
	s.Defaults()
	r := &runner{c: c, s: s, rng: rand.New(rand.NewSource(s.Seed))}
	r.zones = applyControls(c.Zones, s)
	r.byClust = map[campus.Cluster][]int{}
	for i, z := range r.zones {
		r.byClust[z.Cluster] = append(r.byClust[z.Cluster], i)
	}
	r.used = make([][2]int, len(r.zones))
	r.overflow = make([][2]int, len(r.zones))
	r.res = &Result{Scenario: s, Campus: &campus.Campus{Anchors: c.Anchors, Zones: r.zones, Centres: c.Centres},
		Occupancy: make([][]ZoneStep, Steps), UnmetByStep: make([]int, Steps)}

	vehicles := r.fleet()
	vehicles = append(vehicles, r.visitors()...)

	arrivals := make([][]arrival, Steps)
	departures := make([][]departure, Steps)
	homeReturns := make([][]int, Steps)
	homeLeaves := make([][]int, Steps)

	for i, v := range vehicles {
		if v.home >= 0 {
			r.park(v.home, v.kind, false)
		}
		for _, o := range v.outings {
			if v.home >= 0 {
				homeLeaves[o.arrive-1] = append(homeLeaves[o.arrive-1], i)
			}
			arrivals[o.arrive] = append(arrivals[o.arrive], arrival{i, o.cluster, o.leave})
			if v.home >= 0 && o.leave+1 < Steps {
				homeReturns[o.leave+1] = append(homeReturns[o.leave+1], i)
			}
		}
	}

	for step := 0; step < Steps; step++ {
		for _, d := range departures[step] {
			r.unpark(d.zone, d.kind, d.overflow)
		}
		for _, i := range homeLeaves[step] {
			r.leaveHome(vehicles[i])
		}
		for _, a := range arrivals[step] {
			v := vehicles[a.v]
			zone, walk, ok := r.find(a.cluster, v.kind)
			r.res.Arrivals[v.kind]++
			if ok {
				r.park(zone, v.kind, false)
				r.res.WalkSum[v.kind] += walk
			} else {
				zone = r.nearest(a.cluster)
				r.park(zone, v.kind, true)
				r.res.Unmet[v.kind]++
				r.res.UnmetByStep[step]++
			}
			if a.leave < Steps {
				departures[a.leave] = append(departures[a.leave], departure{zone, v.kind, !ok})
			}
		}
		for _, i := range homeReturns[step] {
			r.park(vehicles[i].home, vehicles[i].kind, false)
		}
		snap := make([]ZoneStep, len(r.zones))
		for z := range r.zones {
			snap[z] = ZoneStep{r.used[z][0], r.used[z][1], r.overflow[z][0], r.overflow[z][1]}
		}
		r.res.Occupancy[step] = snap
	}
	return r.res
}

func applyControls(base []campus.Zone, s Scenario) []campus.Zone {
	closed := map[string]bool{}
	for _, n := range s.ClosedZones {
		closed[n] = true
	}
	zones := make([]campus.Zone, len(base))
	copy(zones, base)
	for i := range zones {
		if closed[zones[i].Name] {
			zones[i].CarCap, zones[i].TwoWCap = 0, 0
		}
		zones[i].CarCap += s.ExtraCarBays[zones[i].Name]
	}
	return zones
}

// park puts a vehicle in a zone. Residential zones absorb any excess as
// overflow because residents park near home whether or not a bay is free.
func (r *runner) park(zone int, kind campus.Vehicle, overflow bool) {
	if !overflow && r.used[zone][kind] >= r.zones[zone].Capacity(kind) {
		overflow = true
	}
	if overflow {
		r.overflow[zone][kind]++
	} else {
		r.used[zone][kind]++
	}
}

func (r *runner) unpark(zone int, kind campus.Vehicle, overflow bool) {
	// a vehicle recorded in a bay may leave while overflow vehicles remain;
	// keep counts non-negative either way
	if overflow && r.overflow[zone][kind] > 0 {
		r.overflow[zone][kind]--
		return
	}
	if r.used[zone][kind] > 0 {
		r.used[zone][kind]--
	} else if r.overflow[zone][kind] > 0 {
		r.overflow[zone][kind]--
	}
}

func (r *runner) leaveHome(v vehicle) {
	z := v.home
	if r.overflow[z][v.kind] > 0 {
		r.overflow[z][v.kind]--
	} else if r.used[z][v.kind] > 0 {
		r.used[z][v.kind]--
	}
}

func (r *runner) destination(cl campus.Cluster) (float64, float64) {
	c := r.c.Centres[cl]
	return c[0] + r.rng.NormFloat64()*60, c[1] + r.rng.NormFloat64()*60
}

// find returns the nearest open zone with a free bay inside the walking limit.
func (r *runner) find(cl campus.Cluster, kind campus.Vehicle) (int, float64, bool) {
	x, y := r.destination(cl)
	limit := r.s.MaxWalkCarM
	if kind == campus.TwoWheeler {
		limit = r.s.MaxWalkTwoWM
	}
	best, bestD := -1, math.Inf(1)
	for i, z := range r.zones {
		if z.Cluster == campus.Home || z.Capacity(kind) == 0 || r.used[i][kind] >= z.Capacity(kind) {
			continue
		}
		d := campus.WalkDistance(x, y, z.X, z.Y)
		if d <= limit && d < bestD {
			best, bestD = i, d
		}
	}
	return best, bestD, best >= 0
}

func (r *runner) nearest(cl campus.Cluster) int {
	ids := r.byClust[cl]
	if len(ids) == 0 {
		return 0
	}
	return ids[r.rng.Intn(len(ids))]
}

// ---------------------------------------------------------------- demand

type purpose struct {
	cluster       campus.Cluster
	p             float64
	arriveH, sdH  float64
	dwellH, dwSdH float64
}

func (r *runner) fleet() []vehicle {
	var out []vehicle
	for _, a := range r.c.Anchors {
		cars := int(math.Round(float64(a.Residents) * a.CarRate * r.s.CarOwnership))
		twoW := int(math.Round(float64(a.Residents) * a.TwoWRate * r.s.TwoWOwnership))
		for i := 0; i < cars; i++ {
			out = append(out, vehicle{kind: campus.Car, home: a.HomeZone, outings: r.plan(a.Kind, campus.Car)})
		}
		for i := 0; i < twoW; i++ {
			out = append(out, vehicle{kind: campus.TwoWheeler, home: a.HomeZone, outings: r.plan(a.Kind, campus.TwoWheeler)})
		}
		r.res.Vehicles[campus.Car] += cars
		r.res.Vehicles[campus.TwoWheeler] += twoW
	}
	return out
}

func workPurposes(k campus.AnchorKind, v campus.Vehicle) []purpose {
	switch k {
	case campus.StudentHall:
		if v == campus.Car {
			return []purpose{{campus.Academic, 0.35, 9.0, 0.6, 6.0, 1.5}}
		}
		return []purpose{{campus.Academic, 0.62, 8.9, 0.5, 6.5, 1.5}}
	case campus.StaffQuarters:
		return []purpose{
			{campus.Academic, 0.52, 9.2, 0.45, 8.0, 0.8},
			{campus.Admin, 0.20, 9.3, 0.3, 7.5, 0.6},
			{campus.Hospital, 0.12, 8.6, 0.6, 8.0, 1.0},
		}
	case campus.GuestHouse:
		return []purpose{{campus.Admin, 0.35, 10.0, 1.0, 3.0, 1.0}, {campus.Academic, 0.35, 10.0, 1.0, 3.0, 1.0}}
	default:
		return []purpose{{campus.Academic, 0.7, 9.8, 1.0, 8.0, 1.5}}
	}
}

var eveningPurposes = []purpose{
	{campus.Market, 0.22, 19.3, 0.8, 1.0, 0.4},
	{campus.Sports, 0.10, 17.3, 0.3, 1.5, 0.4},
}

// plan draws the outings of one resident vehicle for the day.
func (r *runner) plan(k campus.AnchorKind, v campus.Vehicle) []outing {
	var out []outing
	pick := func(ps []purpose, shiftCore bool) *purpose {
		u := r.rng.Float64()
		for i := range ps {
			p := ps[i].p
			if shiftCore && ps[i].cluster == campus.Academic {
				p *= 1 - r.s.CoreCarShift
			}
			if u < p {
				return &ps[i]
			}
			u -= ps[i].p
		}
		return nil
	}
	lastLeave := 0
	if p := pick(workPurposes(k, v), v == campus.Car); p != nil {
		if o, ok := r.draw(*p); ok {
			out = append(out, o)
			lastLeave = o.leave
		}
	}
	if p := pick(eveningPurposes, false); p != nil {
		if o, ok := r.draw(*p); ok && o.arrive > lastLeave+2 {
			out = append(out, o)
		}
	}
	return out
}

func (r *runner) draw(p purpose) (outing, bool) {
	arrive := StepOf(p.arriveH + r.rng.NormFloat64()*p.sdH)
	dwell := math.Max(0.25, p.dwellH+r.rng.NormFloat64()*p.dwSdH)
	leave := StepOf(float64(arrive)*StepMin/60 + dwell)
	if arrive < 1 || leave <= arrive {
		return outing{}, false
	}
	return outing{cluster: p.cluster, arrive: arrive, leave: leave}, true
}

// visitors adds day visitors entering through the gates and event traffic.
func (r *runner) visitors() []vehicle {
	var out []vehicle
	share := []struct {
		cl campus.Cluster
		w  float64
	}{{campus.Admin, 0.3}, {campus.Hospital, 0.3}, {campus.Academic, 0.25}, {campus.Market, 0.15}}
	for i := 0; i < r.s.VisitorCars; i++ {
		u, cl := r.rng.Float64(), campus.Admin
		for _, s := range share {
			if u < s.w {
				cl = s.cl
				break
			}
			u -= s.w
		}
		arrive := StepOf(9 + r.rng.Float64()*8)
		leave := StepOf(float64(arrive)*StepMin/60 + math.Max(0.3, 1.5+r.rng.NormFloat64()*0.6))
		out = append(out, vehicle{kind: campus.Car, home: -1, outings: []outing{{cl, arrive, leave}}})
	}
	for _, e := range r.s.Events {
		add := func(n int, kind campus.Vehicle) {
			for i := 0; i < n; i++ {
				arrive := StepOf(e.ArriveFrom + r.rng.Float64()*(e.ArriveTo-e.ArriveFrom))
				leave := StepOf(float64(arrive)*StepMin/60 + e.DwellH)
				out = append(out, vehicle{kind: kind, home: -1, outings: []outing{{campus.Cluster(e.Cluster), arrive, leave}}})
			}
		}
		add(e.Cars, campus.Car)
		add(e.TwoWheelers, campus.TwoWheeler)
	}
	return out
}

// ---------------------------------------------------------------- summaries

// ClusterPeak is the busiest moment of a destination cluster.
type ClusterPeak struct {
	Cluster      campus.Cluster `json:"cluster"`
	PeakTime     string         `json:"peak_time"`
	CarsParked   int            `json:"cars_parked"`
	CarBays      int            `json:"car_bays"`
	Utilisation  float64        `json:"car_utilisation_pct"`
	OverflowCars int            `json:"overflow_cars"`
}

// Clock formats a step as HH:MM.
func Clock(step int) string {
	m := step * StepMin
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// Peaks reports, for every destination cluster, the step with the most parked
// and overflowing cars.
func (res *Result) Peaks() []ClusterPeak {
	clusters := map[campus.Cluster][]int{}
	for i, z := range res.Campus.Zones {
		if z.Cluster != campus.Home {
			clusters[z.Cluster] = append(clusters[z.Cluster], i)
		}
	}
	var out []ClusterPeak
	for cl, ids := range clusters {
		bays := 0
		for _, i := range ids {
			bays += res.Campus.Zones[i].CarCap
		}
		best := ClusterPeak{Cluster: cl, CarBays: bays}
		bestLoad := -1
		for s, snap := range res.Occupancy {
			parked, over := 0, 0
			for _, i := range ids {
				parked += snap[i].Cars
				over += snap[i].OverflowCars
			}
			if parked+over > bestLoad {
				bestLoad = parked + over
				best.PeakTime, best.CarsParked, best.OverflowCars = Clock(s), parked, over
			}
		}
		if bays > 0 {
			best.Utilisation = math.Round(1000*float64(best.CarsParked)/float64(bays)) / 10
		}
		out = append(out, best)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Utilisation > out[j].Utilisation })
	return out
}

// MeanWalk is the average walk from bay to destination for parked arrivals.
func (res *Result) MeanWalk(v campus.Vehicle) float64 {
	n := res.Arrivals[v] - res.Unmet[v]
	if n == 0 {
		return 0
	}
	return res.WalkSum[v] / float64(n)
}

// UnmetShare is the share of destination arrivals that found no bay.
func (res *Result) UnmetShare(v campus.Vehicle) float64 {
	if res.Arrivals[v] == 0 {
		return 0
	}
	return float64(res.Unmet[v]) / float64(res.Arrivals[v])
}

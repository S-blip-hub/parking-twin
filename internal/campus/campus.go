// Package campus defines the accommodation anchors, destination clusters and
// parking zones of a schematic residential campus. Coordinates are metres on a
// local grid. The layout and numbers are illustrative, chosen to be plausible
// for a large Indian residential campus rather than surveyed.
package campus

import (
	"fmt"
	"math"
	"strings"
)

// Vehicle types tracked by the model.
type Vehicle int

const (
	Car Vehicle = iota
	TwoWheeler
)

func (v Vehicle) String() string {
	if v == Car {
		return "car"
	}
	return "two_wheeler"
}

// AnchorKind groups accommodation by who lives there.
type AnchorKind string

const (
	StudentHall   AnchorKind = "student_hall"
	StaffQuarters AnchorKind = "staff_quarters"
	GuestHouse    AnchorKind = "guest_house"
	ScholarFlats  AnchorKind = "scholar_flats"
)

// Anchor is a place where vehicles are kept overnight.
type Anchor struct {
	ID        int
	Name      string
	Kind      AnchorKind
	X, Y      float64
	Residents int
	CarRate   float64 // cars per resident (or household for staff quarters)
	TwoWRate  float64 // two-wheelers per resident
	HomeZone  int     // index of the zone attached to the anchor
}

// Cluster is a group of destinations people drive to.
type Cluster string

const (
	Academic   Cluster = "academic"
	Admin      Cluster = "admin"
	Hospital   Cluster = "hospital"
	Market     Cluster = "market"
	Sports     Cluster = "sports"
	Auditorium Cluster = "auditorium"
	Gate       Cluster = "gate"
	Home       Cluster = "residential"
)

// Zone is a parking area with separate car and two-wheeler bays.
type Zone struct {
	ID       int
	Name     string
	Cluster  Cluster
	X, Y     float64
	CarCap   int
	TwoWCap  int
	AnchorID int // -1 for destination zones
}

// Capacity returns the number of bays for a vehicle type.
func (z Zone) Capacity(v Vehicle) int {
	if v == Car {
		return z.CarCap
	}
	return z.TwoWCap
}

// Campus bundles anchors, zones and destination centres.
type Campus struct {
	Anchors []Anchor
	Zones   []Zone
	Centres map[Cluster][2]float64
}

// WalkDistance is the walking distance between two points, using a detour
// factor of 1.25 over the straight line.
func WalkDistance(x1, y1, x2, y2 float64) float64 {
	return 1.25 * math.Hypot(x1-x2, y1-y2)
}

type anchorSpec struct {
	kind              AnchorKind
	count             int
	residents         [2]int
	carRate, twoWRate float64
	carBays, twoWBays float64 // bays per vehicle owned
}

var anchorSpecs = []anchorSpec{
	{StudentHall, 14, [2]int{420, 760}, 0.02, 0.22, 1.0, 1.0},
	{StaffQuarters, 10, [2]int{60, 160}, 0.75, 0.55, 0.9, 1.0},
	{GuestHouse, 3, [2]int{40, 90}, 0.30, 0.05, 1.2, 1.0},
	{ScholarFlats, 3, [2]int{120, 220}, 0.15, 0.50, 1.0, 1.0},
}

type clusterSpec struct {
	cluster         Cluster
	zones           int
	centre          [2]float64
	spread          float64
	carCap, twoWCap int
}

var clusterSpecs = []clusterSpec{
	{Academic, 12, [2]float64{1000, 950}, 260, 55, 260},
	{Admin, 3, [2]float64{1000, 600}, 90, 70, 90},
	{Hospital, 3, [2]float64{450, 850}, 80, 45, 70},
	{Market, 4, [2]float64{1450, 1350}, 110, 30, 110},
	{Sports, 3, [2]float64{500, 450}, 120, 25, 90},
	{Auditorium, 2, [2]float64{1300, 550}, 60, 140, 80},
	{Gate, 3, [2]float64{1000, 150}, 200, 110, 30},
}

// New builds the base campus: 30 accommodation anchors and 60 parking zones
// (one residential zone per anchor plus 30 destination zones).
func New() *Campus {
	c := &Campus{Centres: map[Cluster][2]float64{}}

	// Anchors sit on a ring around the academic core; staff quarters and guest
	// houses on the south-east, student halls on the north and west.
	idx := 0
	for _, s := range anchorSpecs {
		for i := 0; i < s.count; i++ {
			var x, y float64
			switch s.kind {
			case StudentHall:
				a := math.Pi*0.15 + float64(i)/float64(s.count-1)*math.Pi*1.05
				x, y = 1000+880*math.Cos(a), 1000+820*math.Sin(a)
			case StaffQuarters:
				a := -math.Pi*0.05 - float64(i)/float64(s.count-1)*math.Pi*0.55
				x, y = 1000+900*math.Cos(a), 1000+850*math.Sin(a)
			case GuestHouse:
				x, y = 1250+float64(i)*140, 250
			case ScholarFlats:
				x, y = 150+float64(i)*120, 1250+float64(i)*60
			}
			span := s.residents[1] - s.residents[0]
			residents := s.residents[0] + (span*((idx*37)%11))/10
			a := Anchor{ID: idx, Name: anchorName(s.kind, i), Kind: s.kind, X: round1(x), Y: round1(y),
				Residents: residents, CarRate: s.carRate, TwoWRate: s.twoWRate}
			cars := float64(residents) * s.carRate
			twoW := float64(residents) * s.twoWRate
			z := Zone{ID: len(c.Zones), Name: a.Name + " parking", Cluster: Home, X: a.X + 30, Y: a.Y,
				CarCap: int(math.Round(cars * s.carBays)), TwoWCap: int(math.Round(twoW * s.twoWBays)), AnchorID: a.ID}
			a.HomeZone = z.ID
			c.Anchors = append(c.Anchors, a)
			c.Zones = append(c.Zones, z)
			idx++
		}
	}

	for _, s := range clusterSpecs {
		c.Centres[s.cluster] = s.centre
		for i := 0; i < s.zones; i++ {
			// golden-angle spiral keeps zones evenly spread around the centre
			r := s.spread * math.Sqrt((float64(i)+0.5)/float64(s.zones))
			a := float64(i) * 2.39996
			c.Zones = append(c.Zones, Zone{
				ID: len(c.Zones), Name: fmt.Sprintf("%s P%d", title(s.cluster), i+1), Cluster: s.cluster,
				X: round1(s.centre[0] + r*math.Cos(a)), Y: round1(s.centre[1] + r*math.Sin(a)),
				CarCap: s.carCap, TwoWCap: s.twoWCap, AnchorID: -1,
			})
		}
	}
	return c
}

func anchorName(k AnchorKind, i int) string {
	switch k {
	case StudentHall:
		return fmt.Sprintf("Hall %d", i+1)
	case StaffQuarters:
		return fmt.Sprintf("Staff Quarters %c", 'A'+i)
	case GuestHouse:
		return fmt.Sprintf("Guest House %d", i+1)
	default:
		return fmt.Sprintf("Scholars' Flats %d", i+1)
	}
}

func title(c Cluster) string {
	s := string(c)
	return strings.ToUpper(s[:1]) + s[1:]
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// ZoneIndex returns zone indices grouped by cluster.
func (c *Campus) ZoneIndex() map[Cluster][]int {
	out := map[Cluster][]int{}
	for i, z := range c.Zones {
		out[z.Cluster] = append(out[z.Cluster], i)
	}
	return out
}

package sim

import (
	"encoding/json"
	"fmt"
	"os"
)

// Event is a one-off demand surge such as a convocation or a fest.
type Event struct {
	Name        string  `json:"name"`
	Cluster     string  `json:"cluster"`
	Cars        int     `json:"cars"`
	TwoWheelers int     `json:"two_wheelers"`
	ArriveFrom  float64 `json:"arrive_from_h"`
	ArriveTo    float64 `json:"arrive_to_h"`
	DwellH      float64 `json:"dwell_h"`
}

// Scenario holds every control a planner can change between runs.
type Scenario struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	CarOwnership  float64        `json:"car_ownership_multiplier"`
	TwoWOwnership float64        `json:"two_wheeler_ownership_multiplier"`
	VisitorCars   int            `json:"visitor_cars"`
	MaxWalkCarM   float64        `json:"max_walk_car_m"`
	MaxWalkTwoWM  float64        `json:"max_walk_two_wheeler_m"`
	CoreCarShift  float64        `json:"core_car_shift"`
	ClosedZones   []string       `json:"closed_zones"`
	ExtraCarBays  map[string]int `json:"extra_car_bays"`
	Events        []Event        `json:"events"`
	Seed          int64          `json:"seed"`
}

// Defaults fills unset controls with base values.
func (s *Scenario) Defaults() {
	if s.CarOwnership == 0 {
		s.CarOwnership = 1
	}
	if s.TwoWOwnership == 0 {
		s.TwoWOwnership = 1
	}
	if s.MaxWalkCarM == 0 {
		s.MaxWalkCarM = 400
	}
	if s.MaxWalkTwoWM == 0 {
		s.MaxWalkTwoWM = 250
	}
	if s.Seed == 0 {
		s.Seed = 42
	}
}

// Validate rejects controls that would make a run meaningless.
func (s Scenario) Validate() error {
	switch {
	case s.Name == "":
		return fmt.Errorf("scenario without a name")
	case s.CarOwnership < 0 || s.TwoWOwnership < 0:
		return fmt.Errorf("%s: ownership multipliers must be positive", s.Name)
	case s.CoreCarShift < 0 || s.CoreCarShift > 1:
		return fmt.Errorf("%s: core_car_shift must be between 0 and 1", s.Name)
	}
	for _, e := range s.Events {
		if e.ArriveTo < e.ArriveFrom || e.DwellH <= 0 {
			return fmt.Errorf("%s: event %q has an invalid time window", s.Name, e.Name)
		}
	}
	return nil
}

// LoadScenarios reads a JSON array of scenarios.
func LoadScenarios(path string) ([]Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Scenario
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range list {
		list[i].Defaults()
		if err := list[i].Validate(); err != nil {
			return nil, err
		}
	}
	return list, nil
}

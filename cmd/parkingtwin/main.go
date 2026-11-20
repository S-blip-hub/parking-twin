// Command parkingtwin runs the parking scenarios and writes their results.
//
//	go run ./cmd/parkingtwin -scenarios scenarios/scenarios.json -out outputs
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/S-blip-hub/parking-twin/internal/campus"
	"github.com/S-blip-hub/parking-twin/internal/report"
	"github.com/S-blip-hub/parking-twin/internal/sim"
)

func main() {
	scenarioFile := flag.String("scenarios", "scenarios/scenarios.json", "JSON file with the scenarios to run")
	outDir := flag.String("out", "outputs", "output folder")
	only := flag.String("only", "", "run a single scenario by name")
	flag.Parse()

	if err := run(*scenarioFile, *outDir, *only); err != nil {
		fmt.Fprintln(os.Stderr, "parkingtwin:", err)
		os.Exit(1)
	}
}

func run(scenarioFile, outDir, only string) error {
	scenarios, err := sim.LoadScenarios(scenarioFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	c := campus.New()
	if err := report.WriteLayout(outDir, c); err != nil {
		return err
	}

	var summaries []report.Summary
	for _, s := range scenarios {
		if only != "" && s.Name != only {
			continue
		}
		res := sim.Run(c, s)
		sum, err := report.WriteRun(filepath.Join(outDir, s.Name), res)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
		summaries = append(summaries, sum)
	}
	if len(summaries) == 0 {
		return fmt.Errorf("no scenario named %q", only)
	}
	report.Print(summaries)
	return report.WriteComparison(filepath.Join(outDir, "scenario_comparison.csv"), summaries)
}

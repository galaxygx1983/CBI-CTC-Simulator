// cmd/ctc-sim/scenario.go
package main

import (
	"fmt"
	"os"

	"ctc-simulator/internal/scenario"

	"github.com/spf13/cobra"
)

var (
	scenarioFile    string
	scenarioList     bool
	scenarioValidate bool
)

func init() {
	scenarioCmd.Flags().StringVarP(&scenarioFile, "file", "f", "", "Scenario file path")
	scenarioCmd.Flags().BoolVarP(&scenarioList, "list", "l", false, "List available scenarios")
	scenarioCmd.Flags().BoolVarP(&scenarioValidate, "validate", "v", false, "Validate scenario file")
}

var scenarioCmd = &cobra.Command{
	Use:   "scenario",
	Short: "Manage scenario scripts",
	Long: `Manage scenario scripts for CTC simulator.

Examples:
  ctc-sim scenario --list
  ctc-sim scenario --file scenarios/basic_control.json --validate`,
	Run: runScenario,
}

func runScenario(cmd *cobra.Command, args []string) {
	if scenarioList {
		listScenarios()
		return
	}

	if scenarioFile != "" {
		if scenarioValidate {
			validateScenario(scenarioFile)
		}
		return
	}

	cmd.Help()
}

func listScenarios() {
	fmt.Println("Available scenarios:")
	fmt.Println("  - scenarios/basic_control.json")
}

func validateScenario(path string) {
	script, err := scenario.LoadScript(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Scenario: %s\n", script.Name)
	fmt.Printf("Description: %s\n", script.Description)
	fmt.Printf("Actions: %d\n", len(script.Actions))

	for i, action := range script.Actions {
		fmt.Printf("  %d. %s (delay=%dms, device=%d, cmd=%s)\n",
			i+1, action.Type, action.DelayMs, action.DeviceIndex, action.Command)
	}

	fmt.Println("Validation passed!")
}
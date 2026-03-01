package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show agent status",
	Long:  "Display the current status of all agents from the state store.",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("--config flag is required")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := state.NewFileStore(cfg.State.Dir)
	if err != nil {
		return fmt.Errorf("opening state store: %w", err)
	}

	states, err := store.List(context.Background())
	if err != nil {
		return fmt.Errorf("listing states: %w", err)
	}

	if len(states) == 0 {
		fmt.Println("No agent state found. Agents may not have started yet.")
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	for _, s := range states {
		if s.State == state.StateCreativeThink {
			fmt.Printf("[%s] Creative thinking — generating improvement suggestions\n", s.AgentType)
		}
		if s.State == state.StateDecompose {
			fmt.Printf("[%s] Decomposing — breaking issue into subtasks\n", s.AgentType)
		}
		if err := enc.Encode(s); err != nil {
			return fmt.Errorf("encoding state: %w", err)
		}
	}

	return nil
}

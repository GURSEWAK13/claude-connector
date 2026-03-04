package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/GURSEWAK13/claude-connector/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of a running claude-connector instance",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://localhost:%d/api/status", cfg.Web.Port)

	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-connector is not running (could not reach %s)\n", url)
		fmt.Fprintf(os.Stderr, "Start it with:  ./claude-connector start\n")
		os.Exit(1)
	}
	defer resp.Body.Close()

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(data))
	return nil
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	reportFile      string
	reportName      string
	reportNamespace string
	reportScope     string
	reportRegion    string
	reportFrom      string
	reportTo        string
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generates or applies a time-bounded EmissionReport to the cluster",
	Run: func(cmd *cobra.Command, args []string) {
		generatedKubeconfig := fmt.Sprintf("%s.kubeconfig", clusterName)
		absKubeconfig, _ := filepath.Abs(generatedKubeconfig)
		env := os.Environ()
		if _, err := os.Stat(absKubeconfig); err == nil {
			env = append(env, fmt.Sprintf("KUBECONFIG=%s", absKubeconfig))
		}

		var execCmd *exec.Cmd

		if reportFile != "" {
			fmt.Printf("📄 Applying EmissionReport from file: %s\n", reportFile)
			execCmd = exec.Command("kubectl", "apply", "-f", reportFile)
		} else {
			fmt.Printf("📝 Generating '%s' EmissionReport for region '%s'...\n", reportScope, reportRegion)
			fmt.Printf("🕒 Time Window: %s to %s\n", reportFrom, reportTo)

			yamlTemplate := `apiVersion: sustainability.susk8s/v1alpha1
kind: EmissionReport
metadata:
  name: %s
  namespace: %s
spec:
  scope: "%s"
  region: "%s"
  from: "%s"
  to: "%s"
`
			yamlContent := fmt.Sprintf(yamlTemplate, reportName, reportNamespace, reportScope, reportRegion, reportFrom, reportTo)
			execCmd = exec.Command("kubectl", "apply", "-f", "-")
			execCmd.Stdin = strings.NewReader(yamlContent)
		}

		execCmd.Env = env
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			fmt.Printf("Failed to apply EmissionReport: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("EmissionReport successfully applied! Check your Operator logs to see the audit.")
	},
}

func init() {
	reportCmd.Flags().StringVarP(&clusterName, "cluster", "c", "susk8s-cluster", "Target KinD cluster name")

	reportCmd.Flags().StringVarP(&reportFile, "file", "f", "", "Path to an EmissionReport YAML file")

	// Default timestamps (Now, and 1 Hour ago for quick live-demo feedback!)
	now := time.Now().UTC()
	fiveMinsAgo := now.Add(-1 * time.Hour)

	reportCmd.Flags().StringVarP(&reportName, "report-name", "r", "demo-audit", "Name of the EmissionReport CR")
	reportCmd.Flags().StringVarP(&reportNamespace, "namespace", "s", "default", "Namespace for the report")
	reportCmd.Flags().StringVar(&reportScope, "scope", "namespace", "Scope of the audit (e.g., namespace, workload)")
	reportCmd.Flags().StringVar(&reportRegion, "region", "IE", "Target region for carbon calculation")
	reportCmd.Flags().StringVar(&reportFrom, "from", fiveMinsAgo.Format(time.RFC3339), "Start time (RFC3339 format)")
	reportCmd.Flags().StringVar(&reportTo, "to", now.Format(time.RFC3339), "End time (RFC3339 format)")

	rootCmd.AddCommand(reportCmd)
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launches the dashboards (Kube-Ops-View and Prometheus) in the background",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting UI port-forwards...")

		// Setup Environment
		generatedKubeconfig := fmt.Sprintf("%s.kubeconfig", clusterName)
		absKubeconfig, _ := filepath.Abs(generatedKubeconfig)
		env := os.Environ()
		env = append(env, fmt.Sprintf("KUBECONFIG=%s", absKubeconfig))

		waitForUIComponents(absKubeconfig)

		// Define background processes
		opsViewCmd := exec.Command("kubectl", "port-forward", "-n", "default", "svc/my-ops-view-kube-ops-view", "8080:80")
		opsViewCmd.Env = env

		promCmd := exec.Command("kubectl", "port-forward", "-n", "monitoring", "svc/prometheus-kube-prometheus-prometheus", "9090:9090")
		promCmd.Env = env

		grafanaCmd := exec.Command("kubectl", "port-forward", "-n", "monitoring", "svc/prometheus-grafana", "3000:80")
		grafanaCmd.Env = env

		// Start them in the background
		if err := opsViewCmd.Start(); err != nil {
			fmt.Printf("Failed to start Kube-Ops-View port-forward: %v\n", err)
			os.Exit(1)
		}
		if err := promCmd.Start(); err != nil {
			fmt.Printf("Failed to start Prometheus port-forward: %v\n", err)
			os.Exit(1)
		}
		if err := grafanaCmd.Start(); err != nil {
			fmt.Printf("Failed to start Grafana port-forward: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n=================================================")
		fmt.Println("Port-forwards established successfully!")
		fmt.Println("Kube-Ops-View: http://localhost:8080")
		fmt.Println("Prometheus:    http://localhost:9090")
		fmt.Println("Grafana:       http://localhost:3000")
		fmt.Println("-------------------------------------------------")
		fmt.Println("Dashboards are live! Press [Ctrl+C] to stop.")
		fmt.Println("=================================================")

		// Keep the CLI running until the user presses Ctrl+C
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		// Clean up the background processes
		fmt.Println("\nShutting down port-forwards...")
		opsViewCmd.Process.Kill()
		promCmd.Process.Kill()
		fmt.Println("Goodbye!")
	},
}

func waitForUIComponents(kubeconfigPath string) {
	fmt.Println("Waiting for UI components to be fully Ready (this may take a minute)...")

	env := os.Environ()
	env = append(env, fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	// The workloads to wait for
	targets := [][]string{
		{"default", "deployment/my-ops-view-kube-ops-view"},
		{"monitoring", "deployment/prometheus-grafana"},
		{"monitoring", "statefulset/prometheus-kube-prometheus-prometheus"},
	}

	for _, target := range targets {
		ns := target[0]
		resource := target[1]

		fmt.Printf("   -> Waiting for %s...\n", resource)

		// "kubectl rollout status" natively blocks until the resource is ready
		cmd := exec.Command("kubectl", "rollout", "status", resource, "-n", ns, "--timeout=180s")
		cmd.Env = env

		// If it times out or fails, warn the user
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: Timed out waiting for %s. Port-forward might fail.\n", resource)
			fmt.Printf("   Debug info: %s\n", string(output))
		}
	}
	fmt.Println("All UI components are Ready!")
}

func init() {
	uiCmd.Flags().StringVarP(&clusterName, "name", "n", "susk8s-cluster", "Name of the target KinD cluster")
	rootCmd.AddCommand(uiCmd)
}

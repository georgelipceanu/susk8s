package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Gracefully uninstalls Helm charts, destroys the cluster, and cleans up credentials",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Initiating teardown for '%s'...\n", clusterName)

		// Resolve the kubeconfig path
		generatedKubeconfig := fmt.Sprintf("%s.kubeconfig", clusterName)
		absKubeconfig, err := filepath.Abs(generatedKubeconfig)

		env := os.Environ()
		kubeconfigExists := false

		if err == nil {
			if _, err := os.Stat(absKubeconfig); err == nil {
				kubeconfigExists = true
				env = append(env, fmt.Sprintf("KUBECONFIG=%s", absKubeconfig))
				fmt.Printf("Found cluster credentials: %s\n", absKubeconfig)
			}
		}

		// Gracefully uninstall Helm charts oly if the credentials are here
		if kubeconfigExists {
			fmt.Println("Gracefully uninstalling Helm releases...")

			releases := []struct{ name, namespace string }{
				{"my-ops-view", "default"},
				{"kepler", "kepler"},
				{"prometheus", "monitoring"},
			}

			for _, rel := range releases {
				fmt.Printf("   - Uninstalling %s...\n", rel.name)
				helmCmd := exec.Command("helm", "uninstall", rel.name, "--namespace", rel.namespace)
				helmCmd.Env = env
				helmCmd.Run()
			}
			fmt.Println("Helm uninstalls complete.")
		} else {
			fmt.Println("Kubeconfig not found. Skipping Helm uninstalls and proceeding to cluster deletion.")
		}

		// Destroy the cluster
		fmt.Println("Destroying KinD virtual nodes...")
		kindCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
		kindCmd.Stdout = os.Stdout
		kindCmd.Stderr = os.Stderr

		if err := kindCmd.Run(); err != nil {
			fmt.Printf("Error deleting KinD cluster: %v\n", err)
		} else {
			fmt.Println("Cluster destroyed successfully.")
		}

		// Clean up the local kubeconf
		if kubeconfigExists {
			fmt.Printf("Removing local credential file: %s\n", generatedKubeconfig)
			os.Remove(absKubeconfig)
		}

		fmt.Println("Teardown complete! Your environment is clean.")
	},
}

func init() {
	deleteCmd.Flags().StringVarP(&clusterName, "name", "n", "susk8s-cluster", "Name of the KinD cluster to delete")
	rootCmd.AddCommand(deleteCmd)
}

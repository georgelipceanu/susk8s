package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var kubeconfig string

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Installs the full susk8s stack onto the active Kubernetes cluster",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Installing susk8s stack onto the targeted cluster...")

		env := os.Environ()
		if kubeconfig != "" {
			// Convert the relative path to an absolute path so Makefiles don't break
			absKubeconfig, err := filepath.Abs(kubeconfig)
			if err != nil {
				fmt.Printf("Error resolving kubeconfig path: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("🔌 Targeting custom kubeconfig: %s\n", absKubeconfig)
			env = append(env, fmt.Sprintf("KUBECONFIG=%s", absKubeconfig))
		}

		steps := []struct {
			name string
			cmd  *exec.Cmd
		}{
			// Prerequisites
			{"Installing Cert-Manager", exec.Command("kubectl", "apply", "-f", "https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml")},
			{"Waiting for Cert-Manager Webhook", exec.Command("kubectl", "wait", "--for=condition=Available", "deployment/cert-manager-webhook", "-n", "cert-manager", "--timeout=120s")},

			// Add Helm Repos
			{"Adding Prometheus Repo", exec.Command("helm", "repo", "add", "prometheus-community", "https://prometheus-community.github.io/helm-charts")},
			{"Adding Kepler Repo", exec.Command("helm", "repo", "add", "kepler", "https://sustainable-computing-io.github.io/kepler-helm-chart")},
			{"Adding Kube-Ops-View Repo", exec.Command("helm", "repo", "add", "christianhuth", "https://charts.christianhuth.de")},
			{"Updating Helm Repos", exec.Command("helm", "repo", "update")},

			// Install Observability Stack
			{"Installing Prometheus Stack", exec.Command("helm", "upgrade", "--install", "prometheus", "prometheus-community/kube-prometheus-stack", "--namespace", "monitoring", "--create-namespace", "--set", "prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false")},
			{"Installing Kepler", exec.Command("helm", "upgrade", "--install", "kepler", "kepler/kepler", "--namespace", "kepler", "--create-namespace", "--set", "provider=kind")},
			{"Applying Kepler Services", exec.Command("kubectl", "apply", "-f", "../demo-yamls/kepler-svc.yaml")},
			{"Applying Kepler ServiceMonitors", exec.Command("kubectl", "apply", "-f", "../demo-yamls/kepler-sm.yaml")},
			{"Installing Kube-Ops-View", exec.Command("helm", "upgrade", "--install", "my-ops-view", "christianhuth/kube-ops-view")},

			// Compile and Deploy Operator
			{"Generating Operator Configs", exec.Command("make", "-C", "../operator", "generate")},
			{"Generating Operator Manifests", exec.Command("make", "-C", "../operator", "manifests")},
			{"Installing Operator CRDs", exec.Command("make", "-C", "../operator", "install")},
			{"Building Operator Docker Image", exec.Command("make", "-C", "../operator", "docker-build", "IMG=susk8s-operator:v1")},
			{"Loading Operator Image to KinD", exec.Command("kind", "load", "docker-image", "susk8s-operator:v1", "--name", clusterName)},
			{"Deploying Operator", exec.Command("make", "-C", "../operator", "deploy", "IMG=susk8s-operator:v1")},
			{"Applying susk8s ServiceMonitor for Prometheus", exec.Command("kubectl", "apply", "-f", "../demo-yamls/susk8s-monitor.yaml")},

			// Operator RBAC & Bypasses
			{"Applying Rescheduling RBAC", exec.Command("kubectl", "apply", "-f", "../demo-yamls/rescheduling-rbac.yaml")},
			{"Creating Operator Admin Bypass", exec.Command("bash", "-c", "kubectl create clusterrolebinding operator-admin-bypass --clusterrole=cluster-admin --serviceaccount=operator-system:operator-controller-manager || true")},
			{"Restarting Operator Pods", exec.Command("kubectl", "rollout", "restart", "deployment/operator-controller-manager", "-n", "operator-system")},

			// Compile and Deploy Custom Scheduler
			{"Compiling Custom Scheduler", exec.Command("bash", "-c", "cd ../scheduler && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/kube-scheduler-linux ./cmd/kube-scheduler/main.go")},
			{"Building Scheduler Docker Image", exec.Command("bash", "-c", "cd ../scheduler && docker build -t susk8s-scheduler:v1 .")},
			{"Loading Scheduler Image to KinD", exec.Command("kind", "load", "docker-image", "susk8s-scheduler:v1", "--name", clusterName)},
			{"Applying Scheduler ConfigMap", exec.Command("bash", "-c", "kubectl create configmap susk8s-scheduler-config -n kube-system --from-file=scheduler-config.yaml=../demo-yamls/scheduler-config.yaml --dry-run=client -o yaml | kubectl apply -f -")},
			{"Deploying Custom Scheduler", exec.Command("kubectl", "apply", "-f", "../demo-yamls/scheduler-deploy.yaml")},

			// Apply Grafana Dashboards
			{"Applying susk8s Grafana Dashboard", exec.Command("kubectl", "apply", "-f", "../demo-yamls/grafana-dashboard.yaml")},
		}

		for _, step := range steps {
			fmt.Printf("%s...\n", step.name)

			//  Inject the environment variables into the command BEFORE running it
			step.cmd.Env = env

			// Map Stdout and Stderr to the terminal
			step.cmd.Stdout = os.Stdout
			step.cmd.Stderr = os.Stderr

			if err := step.cmd.Run(); err != nil {
				fmt.Printf("Error during %s: %v\n", step.name, err)
				os.Exit(1)
			}
			fmt.Printf("%s complete.\n\n", step.name)
		}

		fmt.Println("===================================================================")
		fmt.Println("susk8s installation finished successfully!")
		fmt.Println("===================================================================")
		fmt.Println("\nTo view your live cluster metrics, run this command in a separate terminal tab:")
		fmt.Printf("susk8s ui --name=%s \n", clusterName)
		fmt.Println("\nYour custom carbon-aware environment is ready!")
	},
}

func init() {
	installCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file to use for CLI requests")
	installCmd.Flags().StringVarP(&clusterName, "name", "n", "susk8s-cluster", "Name of the targeted KinD cluster")

	rootCmd.AddCommand(installCmd)
}

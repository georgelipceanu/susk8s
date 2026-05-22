//go:build e2e
// +build e2e

/*
Copyright 2025 George Lipceanu.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"susk8s/operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "operator-system"

// serviceAccountName created for the project
const serviceAccountName = "operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
		By("applying the manual RBAC bypasses to the Operator")
		// Apply custom RBAC yaml
		cmd = exec.Command("kubectl", "apply", "-f", "../demo-yamls/rescheduling-rbac.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply manual RBAC yaml")

		// Create the cluster-admin bypass binding
		cmd = exec.Command("kubectl", "create", "clusterrolebinding", "operator-admin-bypass",
			"--clusterrole=cluster-admin",
			"--serviceaccount=operator-system:operator-controller-manager")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster-admin bypass binding")

		// Restart the operator to ensure it boots up with the new God Mode permissions
		cmd = exec.Command("kubectl", "rollout", "restart", "deployment/operator-controller-manager", "-n", "operator-system")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to trigger operator restart")

		// Wait for the restarted operator to come back online
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/operator-controller-manager", "-n", "operator-system", "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Operator failed to roll out after restart")
		By("building and deploying the Custom Scheduler into the cluster")

		// Build the binary and Docker image
		cmd = exec.Command("sh", "-c", "cd ../scheduler && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/kube-scheduler-linux ./cmd/kube-scheduler/main.go && docker build -t susk8s-scheduler:v1 .")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to build Custom Scheduler Docker image")

		// Load the image into the Kind test cluster
		cmd = exec.Command("kind", "load", "docker-image", "susk8s-scheduler:v1", "--name", "operator-test-e2e")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to load Custom Scheduler image into Kind")
		cmd = exec.Command("sh", "-c", "kubectl create configmap susk8s-scheduler-config -n kube-system --from-file=scheduler-config.yaml=../demo-yamls/scheduler-config.yaml --dry-run=client -o yaml | kubectl apply -f -")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Scheduler ConfigMap")
		cmd = exec.Command("kubectl", "apply", "-f", "../demo-yamls/scheduler-deploy.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy Custom Scheduler")

		// Wait for it to be ready before starting the tests
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/susk8s-scheduler", "-n", "kube-system", "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Custom Scheduler failed to roll out")
		By("Waiting for the Kubernetes API server to register the Mutating Webhook")
		time.Sleep(15 * time.Second)
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		Context("Carbon Aware Scheduling & Rescheduling", func() {
			It("Should route pods to the green node and evict them when it becomes dirty", func() {
				By("Finding the worker nodes in the cluster")
				// get all nodes that are not the control-plane
				cmd := exec.Command("kubectl", "get", "nodes",
					"-l", "node-role.kubernetes.io/control-plane!=",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				// split the output to get our two worker nodes
				nodes := strings.Fields(output)
				Expect(len(nodes)).To(BeNumerically(">=", 1), "Need at least one worker node found!")
				greenNode := nodes[0]
				dirtyNode := nodes[0]
				if len(nodes) > 1 {
					dirtyNode = nodes[1]
				}

				By("Setting up initial Carbon Scores on the Nodes via annotations")
				cmd = exec.Command("kubectl", "annotate", "node", greenNode, "susk8s.io/carbon-intensity=50", "--overwrite")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				cmd = exec.Command("kubectl", "annotate", "node", dirtyNode, "susk8s.io/carbon-intensity=900", "--overwrite")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("3. Creating the WorkloadPolicy and Deployment YAML")
				yamlContent := `
apiVersion: sustainability.susk8s/v1alpha1
kind: WorkloadPolicy
metadata:
  name: e2e-policy
  namespace: default
spec:
  enforcement: "hard"
  maxCarbonIntensity: 300
  target:
    matchLabels:
      susk8s.io/tier: ultra-green
  reschedule:
    enabled: true
    mode: "hard"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-deployment
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      susk8s.io/tier: ultra-green
  template:
    metadata:
      labels:
        susk8s.io/tier: ultra-green
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
`
				// Write YAML to a temp file
				yamlFile := filepath.Join("/tmp", "e2e-workload.yaml")
				err = os.WriteFile(yamlFile, []byte(yamlContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				By("Applying the YAML to the cluster")
				cmd = exec.Command("kubectl", "apply", "-f", yamlFile)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the Webhook successfully mutated the pod")
				verifyMutation := func(g Gomega) {
					cmd = exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "susk8s.io/tier=ultra-green", "-o", "jsonpath={.items[0].spec.schedulerName}")
					schedulerName, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(schedulerName).To(ContainSubstring("susk8s-scheduler"))
				}
				Eventually(verifyMutation, 1*time.Minute).Should(Succeed())

				By("Simulating a power grid spike (Green Node becomes Dirty!)")
				cmd = exec.Command("kubectl", "annotate", "node", greenNode, "susk8s.io/carbon-intensity=1000", "--overwrite")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the Rescheduling Controller notices and evicts the pods")
				verifyEviction := func(g Gomega) {
					// check the operator's logs for the eviction action instead of generic Pod events
					cmd = exec.Command("kubectl", "logs", "-n", "operator-system", "-l", "control-plane=controller-manager")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("Evicting pod to force green rescheduling"))
				}
				Eventually(verifyEviction, 2*time.Minute).Should(Succeed())
			})
		})
		It("Should keep pods Pending during a complete brownout (all nodes dirty)", func() {
			By("Finding the worker nodes in the cluster")
			cmd := exec.Command("kubectl", "get", "nodes", "-l", "node-role.kubernetes.io/control-plane!=", "-o", "jsonpath={.items[*].metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			nodes := strings.Fields(output)
			Expect(len(nodes)).To(BeNumerically(">=", 1), "Need at least one worker node found!")

			By("Setting up a 'Brownout' - Annotating ALL nodes as dirty (800)")
			annotateArgs := []string{"annotate", "node"}
			annotateArgs = append(annotateArgs, nodes...)
			annotateArgs = append(annotateArgs, "susk8s.io/carbon-intensity=800", "--overwrite")
			utils.Run(exec.Command("kubectl", annotateArgs...))

			By("Applying the Brownout WorkloadPolicy first")
			policyYAML := `
apiVersion: sustainability.susk8s/v1alpha1
kind: WorkloadPolicy
metadata:
  name: global-brownout-policy
  namespace: default
spec:
  enforcement: "hard"
  maxCarbonIntensity: 300
  target:
    matchLabels:
      scenario: brownout  # Targets the test workload
  reschedule:
    enabled: true
    mode: "hard"
`
			os.WriteFile("/tmp/brownout-policy.yaml", []byte(policyYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/brownout-policy.yaml"))

			// webhook and scheduler need time to cache
			time.Sleep(5 * time.Second)

			By("Applying the Brownout Deployment to the cluster")
			deployYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: brownout-deployment
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      scenario: brownout
  template:
    metadata:
      labels:
        susk8s.io/tier: ultra-green  # Webhook sees this -> injects Scheduler
        scenario: brownout           # Webhook sees this -> injects 300 limit annotation
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
`
			os.WriteFile("/tmp/brownout-deploy.yaml", []byte(deployYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/brownout-deploy.yaml"))

			By("Verifying the Filter Plugin keeps the Pod stuck in Pending state")
			verifyPending := func(g Gomega) {
				cmd = exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=brownout", "-o", "jsonpath={.items[0].status.phase}")
				phase, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// if phase != "Pending" {
				// 	fmt.Println("\n=======================================================")
				// 	fmt.Println(" POD ESCAPED PENDING STATE! DUMPING EVIDENCE ")

				// 	fmt.Println("\n--- POD DESCRIBE (Check Annotations & Events) ---")
				// 	outDesc, _ := utils.Run(exec.Command("kubectl", "describe", "pods", "-n", "default", "-l", "scenario=brownout"))
				// 	fmt.Println(outDesc)

				// 	fmt.Println("\n--- CUSTOM SCHEDULER LOGS ---")
				// 	outLogs, _ := utils.Run(exec.Command("kubectl", "logs", "deployment/susk8s-scheduler", "-n", "kube-system"))
				// 	fmt.Println(outLogs)
				// 	fmt.Println("=======================================================")
				// }
				g.Expect(phase).To(Equal("Pending"))
			}

			Eventually(verifyPending, 30*time.Second).Should(Succeed())
			Consistently(verifyPending, 10*time.Second).Should(Succeed())

			By("Cleaning up")
			exec.Command("kubectl", "delete", "-f", "/tmp/brownout-deploy.yaml").Run()
			exec.Command("kubectl", "delete", "-f", "/tmp/brownout-policy.yaml").Run()
		})
		It("Should enforce multi-tiered policies (Bronze evicts, Platinum survives)", func() {
			By("Finding the worker nodes in the cluster")
			cmd := exec.Command("kubectl", "get", "nodes", "-l", "node-role.kubernetes.io/control-plane!=", "-o", "jsonpath={.items[*].metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			nodes := strings.Fields(output)
			Expect(len(nodes)).To(BeNumerically(">=", 1), "Need at least one worker node!")

			By("Starting the grid in a 'Green' state (100)")
			annotateArgs := []string{"annotate", "node"}
			annotateArgs = append(annotateArgs, nodes...)
			annotateArgs = append(annotateArgs, "susk8s.io/carbon-intensity=100", "--overwrite")
			utils.Run(exec.Command("kubectl", annotateArgs...))

			By("Applying Bronze (150) and Platinum (500) WorkloadPolicies")
			policiesYAML := `
apiVersion: sustainability.susk8s/v1alpha1
kind: WorkloadPolicy
metadata:
  name: bronze-policy
  namespace: default
spec:
  enforcement: "hard"
  maxCarbonIntensity: 150
  target:
    matchLabels:
      scenario: bronze
  reschedule:
    enabled: true
    mode: "hard"
---
apiVersion: sustainability.susk8s/v1alpha1
kind: WorkloadPolicy
metadata:
  name: platinum-policy
  namespace: default
spec:
  enforcement: "hard"
  maxCarbonIntensity: 500
  target:
    matchLabels:
      scenario: platinum
  reschedule:
    enabled: true
    mode: "hard"
`
			os.WriteFile("/tmp/multi-tier-policies.yaml", []byte(policiesYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/multi-tier-policies.yaml"))
			time.Sleep(5 * time.Second) // Let caches sync

			By("Deploying Bronze and Platinum Workloads")
			deploysYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bronze-deployment
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      scenario: bronze
  template:
    metadata:
      labels:
        susk8s.io/tier: ultra-green
        scenario: bronze
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: platinum-deployment
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      scenario: platinum
  template:
    metadata:
      labels:
        susk8s.io/tier: ultra-green
        scenario: platinum
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
`
			os.WriteFile("/tmp/multi-tier-deploys.yaml", []byte(deploysYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/multi-tier-deploys.yaml"))

			By("Waiting for BOTH pods to become Running")
			verifyBothRunning := func(g Gomega) {
				bronzePhase, _ := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=bronze", "-o", "jsonpath={.items[0].status.phase}"))
				platinumPhase, _ := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=platinum", "-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(bronzePhase).To(Equal("Running"))
				g.Expect(platinumPhase).To(Equal("Running"))
			}
			Eventually(verifyBothRunning, 60*time.Second).Should(Succeed())

			By("Spiking the grid to 'Moderate' (300), exceeding Bronze, but below Platinum")
			annotateArgs300 := []string{"annotate", "node"}
			annotateArgs300 = append(annotateArgs300, nodes...)
			annotateArgs300 = append(annotateArgs300, "susk8s.io/carbon-intensity=300", "--overwrite")
			utils.Run(exec.Command("kubectl", annotateArgs300...))

			By("Verifying Operator evicts Bronze and the new Pod gets stuck Pending")
			verifyBronzePending := func(g Gomega) {
				// Because the Operator deletes the pod, a new one spins up and hits the Filter
				bronzePhase, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=bronze", "-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(bronzePhase).To(Equal("Pending"))
			}
			Eventually(verifyBronzePending, 45*time.Second).Should(Succeed())

			By("Verifying Platinum remains blissfully Running")
			verifyPlatinumRunning := func(g Gomega) {
				platinumPhase, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=platinum", "-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(platinumPhase).To(Equal("Running"))
			}
			Consistently(verifyPlatinumRunning, 10*time.Second).Should(Succeed())

			By("Cleaning up")
			exec.Command("kubectl", "delete", "-f", "/tmp/multi-tier-deploys.yaml").Run()
			exec.Command("kubectl", "delete", "-f", "/tmp/multi-tier-policies.yaml").Run()
		})
		It("Should schedule a Pending pod when the sun comes out (Node Recovery)", func() {
			By("Finding the worker nodes in the cluster")
			cmd := exec.Command("kubectl", "get", "nodes", "-l", "node-role.kubernetes.io/control-plane!=", "-o", "jsonpath={.items[*].metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			nodes := strings.Fields(output)
			Expect(len(nodes)).To(BeNumerically(">=", 1), "Need at least one worker node!")

			By("Setting up a total Brownout (800)")
			annotateArgs := []string{"annotate", "node"}
			annotateArgs = append(annotateArgs, nodes...)
			annotateArgs = append(annotateArgs, "susk8s.io/carbon-intensity=800", "--overwrite")
			utils.Run(exec.Command("kubectl", annotateArgs...))

			By("Applying Policy (max 300)")
			policyYAML := `
apiVersion: sustainability.susk8s/v1alpha1
kind: WorkloadPolicy
metadata:
  name: recovery-policy
  namespace: default
spec:
  enforcement: "hard"
  maxCarbonIntensity: 300
  target:
    matchLabels:
      scenario: recovery
  reschedule:
    enabled: true
    mode: "hard"
`
			os.WriteFile("/tmp/recovery-policy.yaml", []byte(policyYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/recovery-policy.yaml"))
			time.Sleep(5 * time.Second) // Cache sync

			By("Deploying the workload")
			deployYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: recovery-deployment
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      scenario: recovery
  template:
    metadata:
      labels:
        susk8s.io/tier: ultra-green
        scenario: recovery
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
`
			os.WriteFile("/tmp/recovery-deploy.yaml", []byte(deployYAML), 0644)
			utils.Run(exec.Command("kubectl", "apply", "-f", "/tmp/recovery-deploy.yaml"))

			By("Verifying the pod is stuck Pending due to the brownout")
			verifyPending := func(g Gomega) {
				phase, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=recovery", "-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Pending"))
			}
			Eventually(verifyPending, 30*time.Second).Should(Succeed())
			Consistently(verifyPending, 5*time.Second).Should(Succeed()) // Ensure it stays stuck

			By("THE SUN COMES OUT: Annotating one node as 'Green' (50)")
			targetNode := nodes[0] // Pick the first worker
			utils.Run(exec.Command("kubectl", "annotate", "node", targetNode, "susk8s.io/carbon-intensity=50", "--overwrite"))

			By("Verifying the Custom Scheduler wakes up and runs the Pod")
			verifyRunning := func(g Gomega) {
				phase, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", "default", "-l", "scenario=recovery", "-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(phase).To(Equal("Running"))
			}
			Eventually(verifyRunning, 45*time.Second).Should(Succeed())

			By("Cleaning up")
			exec.Command("kubectl", "delete", "-f", "/tmp/recovery-deploy.yaml").Run()
			exec.Command("kubectl", "delete", "-f", "/tmp/recovery-policy.yaml").Run()
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

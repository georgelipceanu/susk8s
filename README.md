# SusK8s
> Elevating energy and carbon emissions to first-class operational signals in Kubernetes.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.30+-326CE5?style=flat&logo=kubernetes)](https://kubernetes.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
## Getting Started

### Prerequisites
* Go (v1.21+)
* Docker
* Kubernetes CLI (\`kubectl\`)
* Helm
* KinD (Kubernetes in Docker)

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/georgelipceanu/susk8s.git
   cd susk8s
   ```
2. **Build the CLI:**
   ```bash
   cd cli
   go build -o susk8s main.go
   sudo mv susk8s /usr/local/bin/
   ```
3. **Provision the Cluster and/or Install the Stack:**
   ```bash
   susk8s create -n <cluster-name>
   ```

OR if you have a cluster already created:
```bash
   susk8s install --kubeconfig <kubeconfig>
```

See the binary's `help` option for more details on options available
---

## Purpose and Usage

To find a comprehensive report on this project, its purpose and how it is used, see this report [here](https://github.com/georgelipceanu/fyp/blob/main/fyp2_document.pdf)

---

## Future Work

* **Carbon-Aware Cluster Autoscaler:** Integrating the operator with Karpenter to physically scale down idle nodes after workload eviction.
* **Multi-Region Cluster Routing:** Expanding the control plane via Karmada to route incoming traffic globally based on renewable energy availability.

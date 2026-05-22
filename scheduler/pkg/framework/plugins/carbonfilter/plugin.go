package carbonfilter

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const Name = "CarbonFilter"

// CarbonFilter strictly blocks pods from scheduling on nodes
// whose carbon intensity exceeds the pod's requested limit.
type CarbonFilter struct {
	handle framework.Handle
}

var _ framework.FilterPlugin = &CarbonFilter{}

func New(_ context.Context, _ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &CarbonFilter{
		handle: h,
	}, nil
}

func (c *CarbonFilter) Name() string {
	return Name
}

func (c *CarbonFilter) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}

	// Get the Node's current Carbon Score from annotations
	carbonStr, exists := node.Annotations["susk8s.io/carbon-intensity"]
	if !exists {
		// If the node has no score, allow it through
		return framework.NewStatus(framework.Success, "")
	}
	nodeCarbon, err := strconv.Atoi(carbonStr)
	if err != nil {
		return framework.NewStatus(framework.Error, "invalid node carbon score")
	}

	//  Get the Pod's Max Limit from annotations
	limitStr, hasLimit := pod.Annotations["susk8s.io/max-carbon"]
	if !hasLimit {
		// If the pod doesn't have a specific limit, allow it through
		return framework.NewStatus(framework.Success, "")
	}
	podLimit, err := strconv.Atoi(limitStr)
	if err != nil {
		return framework.NewStatus(framework.Error, "invalid pod carbon limit")
	}
	if nodeCarbon > podLimit {
		enforcementMode, hasEnforcement := pod.Annotations["susk8s.io/enforcement"]
		if hasEnforcement && enforcementMode == "soft" {
			// Produce a native Kubernetes Event for Day-2 Observability
			c.handle.EventRecorder().Eventf(
				pod,
				nil, // no related object
				v1.EventTypeWarning,
				"SoftCarbonThresholdBreached",
				"CarbonFilter",
				"Node %q carbon (%d) exceeds pod limit (%d), allowing due to soft enforcement",
				node.Name, nodeCarbon, podLimit,
			)
			// Soft mode allows scheduling to proceed despite the violation
			return framework.NewStatus(framework.Success, "")
		}
		reason := fmt.Sprintf("Node carbon (%d) exceeds pod limit (%d)", nodeCarbon, podLimit)
		return framework.NewStatus(framework.Unschedulable, reason)
	}

	// The node is safely under the limit
	return framework.NewStatus(framework.Success, "")
}

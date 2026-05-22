package carbonbinpack

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const Name = "CarbonBinPack"

// CarbonBinPack is a Score plugin that favors nodes with low carbon intensity
// and high resource utilisation (bin-packing).
type CarbonBinPack struct {
	handle framework.Handle
}

var _ framework.ScorePlugin = &CarbonBinPack{}

func New(_ context.Context, _ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &CarbonBinPack{handle: h}, nil
}

func (pl *CarbonBinPack) Name() string {
	return Name
}

func (pl *CarbonBinPack) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil || nodeInfo.Node() == nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()

	utilWeight := getWeight(pod, "susk8s.io/utilisationWeight", 70) // retrieve policy weights from annotations
	carbonWeight := getWeight(pod, "susk8s.io/carbonWeight", 30)

	carbonScore := calculateCarbonScore(node)    // calculate carbon score
	utilScore := calculateBinPackScore(nodeInfo) // calculate utilisation score
	finalScore := (carbonScore*carbonWeight + utilScore*utilWeight) / (carbonWeight + utilWeight)
	return finalScore, nil
}

func (pl *CarbonBinPack) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func getWeight(pod *v1.Pod, key string, defaultVal int64) int64 { // read weights from pod annotations
	if val, ok := pod.Annotations[key]; ok {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func calculateCarbonScore(node *v1.Node) int64 { // (Low Carbon = High Score)
	val, ok := node.Annotations["susk8s.io/carbon-intensity"]
	if !ok {
		return 50
	}

	intensity, err := strconv.Atoi(val)
	if err != nil {
		return 50
	}

	const maxIntensity = 1000
	if intensity > maxIntensity {
		intensity = maxIntensity
	}
	return int64(100 - (intensity * 100 / maxIntensity)) // map [0, 1000] -> [100, 0]
}

func calculateBinPackScore(nodeInfo *framework.NodeInfo) int64 { // based on CPU/Memory requests
	node := nodeInfo.Node()
	requested := nodeInfo.Requested
	allocatable := node.Status.Allocatable

	cpuScore := (requested.MilliCPU * 100) / allocatable.Cpu().MilliValue()
	memScore := (requested.Memory * 100) / allocatable.Memory().Value()

	return (cpuScore + memScore) / 2
}

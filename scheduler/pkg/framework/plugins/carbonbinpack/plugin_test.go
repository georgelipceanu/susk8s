package carbonbinpack

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func TestGetWeight(t *testing.T) {
	tests := []struct {
		name       string
		pod        *v1.Pod
		key        string
		defaultVal int64
		expected   int64
	}{
		{
			name:       "Missing annotation returns default",
			pod:        &v1.Pod{ObjectMeta: metav1.ObjectMeta{}},
			key:        "susk8s.io/carbonWeight",
			defaultVal: 30,
			expected:   30,
		},
		{
			name: "Valid annotation returns parsed value",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"susk8s.io/carbonWeight": "80"},
				},
			},
			key:        "susk8s.io/carbonWeight",
			defaultVal: 30,
			expected:   80,
		},
		{
			name: "Invalid annotation string returns default",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"susk8s.io/carbonWeight": "not-a-number"},
				},
			},
			key:        "susk8s.io/carbonWeight",
			defaultVal: 30,
			expected:   30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWeight(tt.pod, tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCalculateCarbonScore(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    int64
	}{
		{
			name:        "Missing annotation returns 50",
			annotations: nil,
			expected:    50,
		},
		{
			name:        "Invalid string returns 50",
			annotations: map[string]string{"susk8s.io/carbon-intensity": "invalid"},
			expected:    50,
		},
		{
			name:        "Zero carbon intensity returns max score (100)",
			annotations: map[string]string{"susk8s.io/carbon-intensity": "0"},
			expected:    100,
		},
		{
			name:        "Medium carbon intensity (500) returns half score (50)",
			annotations: map[string]string{"susk8s.io/carbon-intensity": "500"},
			expected:    50,
		},
		{
			name:        "Max carbon intensity (1000) returns minimum score (0)",
			annotations: map[string]string{"susk8s.io/carbon-intensity": "1000"},
			expected:    0,
		},
		{
			name:        "Extremely high intensity (>1000) is clamped to minimum score (0)",
			annotations: map[string]string{"susk8s.io/carbon-intensity": "1500"},
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			result := calculateCarbonScore(node)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCalculateBinPackScore(t *testing.T) {
	tests := []struct {
		name          string
		reqMilliCPU   int64
		reqMemory     int64
		allocMilliCPU int64
		allocMemory   int64
		expected      int64
	}{
		{
			name:          "50% CPU and 50% Memory utilisation",
			reqMilliCPU:   500,
			reqMemory:     500,
			allocMilliCPU: 1000,
			allocMemory:   1000,
			expected:      50, // (50 + 50) / 2
		},
		{
			name:          "100% CPU and 0% Memory utilisation",
			reqMilliCPU:   1000,
			reqMemory:     0,
			allocMilliCPU: 1000,
			allocMemory:   1000,
			expected:      50, // (100 + 0) / 2
		},
		{
			name:          "25% CPU and 75% Memory utilisation",
			reqMilliCPU:   250,
			reqMemory:     750,
			allocMilliCPU: 1000,
			allocMemory:   1000,
			expected:      50, // (25 + 75) / 2
		},
		{
			name:          "100% Utilisation of both",
			reqMilliCPU:   2000,
			reqMemory:     4000,
			allocMilliCPU: 2000,
			allocMemory:   4000,
			expected:      100, // (100 + 100) / 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the fake Node with total allocatable resources
			node := &v1.Node{
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(tt.allocMilliCPU, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(tt.allocMemory, resource.BinarySI),
					},
				},
			}

			// Setup the fake Framework NodeInfo with currently requested resources
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(node)
			nodeInfo.Requested = &framework.Resource{
				MilliCPU: tt.reqMilliCPU,
				Memory:   tt.reqMemory,
			}
			result := calculateBinPackScore(nodeInfo)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

type mockNodeInfoLister struct {
	framework.NodeInfoLister
	nodeInfo *framework.NodeInfo
	err      error
}

func (m *mockNodeInfoLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return m.nodeInfo, m.err
}

type mockSharedLister struct {
	framework.SharedLister
	nodeInfoLister framework.NodeInfoLister
}

func (m *mockSharedLister) NodeInfos() framework.NodeInfoLister {
	return m.nodeInfoLister
}

type mockHandle struct {
	framework.Handle
	sharedLister framework.SharedLister
}

func (m *mockHandle) SnapshotSharedLister() framework.SharedLister {
	return m.sharedLister
}

func TestPluginBasics(t *testing.T) {
	ctx := context.Background()
	dummyHandle := &mockHandle{}

	plugin, err := New(ctx, nil, dummyHandle)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if plugin.Name() != Name {
		t.Errorf("expected name %s, got %s", Name, plugin.Name())
	}

	// Safely cast to ScorePlugin to check extensions
	scorePlugin := plugin.(framework.ScorePlugin)
	if scorePlugin.ScoreExtensions() != nil {
		t.Error("expected ScoreExtensions to return nil")
	}
}

func TestScore(t *testing.T) {
	// Setup a perfect fake Node
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				"susk8s.io/carbon-intensity": "0", // 0 intensity = 100 Carbon Score
			},
		},
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(1000, resource.BinarySI),
			},
		},
	}

	// Setup NodeInfo (500/1000 requested = 50% Utilisation = 50 BinPack Score)
	nodeInfo := framework.NewNodeInfo()
	nodeInfo.SetNode(node)
	nodeInfo.Requested = &framework.Resource{
		MilliCPU: 500,
		Memory:   500,
	}

	// Create the fake Pod requesting default weights
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			// No annotations, so it defaults to 70 weight for util, 30 weight for carbon
		},
	}

	t.Run("Successfully calculates combined score", func(t *testing.T) {
		handle := &mockHandle{
			sharedLister: &mockSharedLister{
				nodeInfoLister: &mockNodeInfoLister{
					nodeInfo: nodeInfo,
					err:      nil,
				},
			},
		}

		plugin := &CarbonBinPack{handle: handle}
		score, status := plugin.Score(context.Background(), nil, pod, "test-node")

		if !status.IsSuccess() {
			t.Errorf("expected success status, got %v", status)
		}
		if score != 65 {
			t.Errorf("expected final score to be 65, got %d", score)
		}
	})

	t.Run("Returns error if node is missing from snapshot", func(t *testing.T) {
		handle := &mockHandle{
			sharedLister: &mockSharedLister{
				nodeInfoLister: &mockNodeInfoLister{
					nodeInfo: nil,
					err:      fmt.Errorf("not found"),
				},
			},
		}

		plugin := &CarbonBinPack{handle: handle}
		_, status := plugin.Score(context.Background(), nil, pod, "missing-node")

		if status.IsSuccess() {
			t.Error("expected error status, got success")
		}
	})
}

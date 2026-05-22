package carbonfilter

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// fakeHandle mocks the framework.Handle interface strictly to provide our fake EventRecorder.
type fakeHandle struct {
	framework.Handle
	recorder events.EventRecorder
}

// EventRecorder overrides the default interface method to return our mock recorder.
func (f *fakeHandle) EventRecorder() events.EventRecorder {
	return f.recorder
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name            string
		podAnnotations  map[string]string
		nodeAnnotations map[string]string
		expectedStatus  *framework.Status
	}{
		{
			name: "Success - Node is green enough (200 <= 300)",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "300",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "200",
			},
			expectedStatus: framework.NewStatus(framework.Success, ""),
		},
		{
			name: "Success - Node perfectly matches limit (300 == 300)",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "300",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "300",
			},
			expectedStatus: framework.NewStatus(framework.Success, ""),
		},
		{
			name: "Unschedulable - Node is too dirty (no enforcement specified)",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "300",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "800",
			},
			expectedStatus: framework.NewStatus(framework.Unschedulable, "Node carbon (800) exceeds pod limit (300)"),
		},
		{
			name: "Unschedulable - Node is too dirty and enforcement is hard",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon":  "300",
				"susk8s.io/enforcement": "hard",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "800",
			},
			expectedStatus: framework.NewStatus(framework.Unschedulable, "Node carbon (800) exceeds pod limit (300)"),
		},
		{
			name: "Success - Node is too dirty but enforcement is soft",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon":  "300",
				"susk8s.io/enforcement": "soft",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "800",
			},
			expectedStatus: framework.NewStatus(framework.Success, ""),
		},
		{
			name: "Success - Node has no carbon score",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "300",
			},
			nodeAnnotations: nil, // Simulates a node missing the annotation
			expectedStatus:  framework.NewStatus(framework.Success, ""),
		},
		{
			name:           "Success - Pod does not care about carbon",
			podAnnotations: nil, // Simulates a pod without the limit annotation
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "800",
			},
			expectedStatus: framework.NewStatus(framework.Success, ""),
		},
		{
			name: "Error - Node has invalid carbon string",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "300",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "not-a-number",
			},
			expectedStatus: framework.NewStatus(framework.Error, "invalid node carbon score"),
		},
		{
			name: "Error - Pod has invalid limit string",
			podAnnotations: map[string]string{
				"susk8s.io/max-carbon": "not-a-number",
			},
			nodeAnnotations: map[string]string{
				"susk8s.io/carbon-intensity": "200",
			},
			expectedStatus: framework.NewStatus(framework.Error, "invalid pod carbon limit"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the Fake Event Recorder and Mock Handle
			fakeRecorder := events.NewFakeRecorder(10)
			mockHandle := &fakeHandle{recorder: fakeRecorder}

			// Setup the Filter Plugin with the injected handle
			filter := &CarbonFilter{
				handle: mockHandle,
			}

			// Setup the Mock Pod
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Annotations: tt.podAnnotations,
				},
			}

			// Setup the Mock Node
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: tt.nodeAnnotations,
				},
			}
			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(node)

			// Run the Filter logic
			status := filter.Filter(context.Background(), nil, pod, nodeInfo)

			// Assert the Results
			if status.Code() != tt.expectedStatus.Code() {
				t.Errorf("expected status code %v, got %v", tt.expectedStatus.Code(), status.Code())
			}

			if status.Message() != tt.expectedStatus.Message() {
				t.Errorf("expected status message %q, got %q", tt.expectedStatus.Message(), status.Message())
			}

			// Specific assertion for the soft event
			if tt.name == "Success - Node is too dirty but enforcement is soft" {
				select {
				case event := <-fakeRecorder.Events:
					expectedEventSubstr := "SoftCarbonThresholdBreached"
					if !contains(event, expectedEventSubstr) {
						t.Errorf("expected event containing %q, got %q", expectedEventSubstr, event)
					}
				default:
					t.Errorf("expected a warning event to be emitted, but none was found")
				}
			}
		})
	}
}

func TestName(t *testing.T) {
	filter := &CarbonFilter{}
	if filter.Name() != Name {
		t.Errorf("expected plugin name to be %q, got %q", Name, filter.Name())
	}
}

func TestNilNode(t *testing.T) {
	filter := &CarbonFilter{}
	pod := &v1.Pod{}

	// empty NodeInfo where Node() returns nil
	nodeInfo := framework.NewNodeInfo()

	status := filter.Filter(context.Background(), nil, pod, nodeInfo)
	if status.Code() != framework.Error {
		t.Errorf("expected Error status for nil node, got %v", status.Code())
	}
}

// Simple helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

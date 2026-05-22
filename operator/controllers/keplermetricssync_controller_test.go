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

package controllers

import (
	"context"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// create a fake client that implements the promapi.Client interface
type MockPrometheusClient struct {
	Response []byte
	Err      error
}

func (m *MockPrometheusClient) URL(ep string, args map[string]string) *url.URL {
	return &url.URL{Scheme: "http", Host: "localhost", Path: ep}
}

func (m *MockPrometheusClient) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	if m.Err != nil {
		return nil, nil, m.Err
	}
	// Return a fake HTTP 200 OK along with our mock JSON response
	return &http.Response{StatusCode: http.StatusOK}, m.Response, nil
}

var _ = Describe("KeplerMetricsSync Controller", func() {
	Context("When reconciling a Node", func() {
		It("Should query Prometheus and update the Node's energy-usage annotation", func() {
			ctx := context.Background()
			nodeName := "test-node-kepler"

			By("1. Creating a test Node")
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: corev1.NodeSpec{},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			By("2. Injecting an InternalIP into the Node Status")
			// The EnvTest cluster doesn't automatically assign IPs to nodes, so we must mock it
			node.Status = corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "192.168.1.100"},
				},
			}
			Expect(k8sClient.Status().Update(ctx, node)).Should(Succeed())

			By("3. Setting up the Mock Prometheus Client with fake Kepler data")
			// This is the exact JSON structure the Prometheus library expects to receive
			mockJSON := []byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [
						{
							"metric": {},
							"value": [1680000000, "15.75"] 
						}
					]
				}
			}`)
			mockClient := &MockPrometheusClient{
				Response: mockJSON,
			}

			By("4. Manually triggering the Reconciler")
			reconciler := &KeplerMetricsSyncReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockClient, // Inject our fake client!
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("5. Verifying the Node received the energy-usage annotation")
			updatedNode := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, updatedNode)).Should(Succeed())

			// Verify the annotation exists and matches our mocked data
			// Note: %f in Go defaults to 6 decimal places, so 15.75 becomes 15.750000
			Expect(updatedNode.Annotations).ToNot(BeNil())
			Expect(updatedNode.Annotations["susk8s.io/energy-usage"]).Should(Equal("15.750000"))

			By("6. Cleaning up")
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())
		})
		It("Should safely log an error and skip the node if Prometheus returns empty data", func() {
			ctx := context.Background()
			nodeName := "empty-data-node"

			By("1. Creating a test Node")
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			node.Status = corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.5"}},
			}
			Expect(k8sClient.Status().Update(ctx, node)).Should(Succeed())

			By("2. Setting up the Mock Prometheus Client to return NO metrics")
			// Notice the "result": [] array is empty!
			mockJSON := []byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [] 
				}
			}`)
			mockClient := &MockPrometheusClient{Response: mockJSON}

			By("3. Triggering the Reconciler")
			reconciler := &KeplerMetricsSyncReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				PrometheusClient: mockClient,
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}}

			// The Reconciler should catch the error internally, log it, and exit without crashing
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("4. Verifying the Node was completely ignored (no annotations added)")
			updatedNode := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, updatedNode)).Should(Succeed())

			// Because there was no data, it should have skipped adding the annotation map entirely
			Expect(updatedNode.Annotations).To(BeNil())

			By("5. Cleaning up")
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())
		})
	})
})

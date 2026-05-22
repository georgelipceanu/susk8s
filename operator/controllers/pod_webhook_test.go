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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

var _ = Describe("PodMutator Webhook", func() {
	var mutator *PodMutator

	BeforeEach(func() {
		// Set up the Kubernetes Decoder so webhook can read the raw JSON
		scheme := runtime.NewScheme()
		err := corev1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		// Add Custom CRD to the scheme so the Fake Client understands it
		err = sustainabilityv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		// Create a Mock WorkloadPolicy in the fake cluster
		mockPolicy := &sustainabilityv1alpha1.WorkloadPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: "default",
			},
			Spec: sustainabilityv1alpha1.WorkloadPolicySpec{
				MaxCarbonIntensity: ptr.To[int32](300),
				Enforcement:        "hard",
				Target: sustainabilityv1alpha1.WorkloadTarget{
					MatchLabels: map[string]string{
						"app": "green-workload",
					},
				},
			},
		}

		// Build the Fake Client with scheme and load the mock policy
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(mockPolicy).
			Build()

		decoder := admission.NewDecoder(scheme)
		mutator = &PodMutator{
			Client:  fakeClient,
			Decoder: decoder,
		}
	})

	Context("When evaluating a Pod creation request", func() {
		It("Should dynamically inject the custom scheduler and annotations IF the pod matches a WorkloadPolicy", func() {
			ctx := context.Background()

			// Create a Pod with the target label defined in our mock policy
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "green-test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": "green-workload",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}

			// Convert it to raw JSON bytes (simulating the API server request)
			rawPod, err := json.Marshal(pod)
			Expect(err).NotTo(HaveOccurred())

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: rawPod},
				},
			}

			// Pass it to your Webhook handler
			resp := mutator.Handle(ctx, req)

			// Verify the results
			Expect(resp.Allowed).To(BeTrue(), "Webhook should always allow the request")

			// expect exactly two patches, one for schedulerName, one for the new annotations map
			Expect(resp.Patches).To(HaveLen(2), "Webhook should generate exactly two patches")

			// Helper variables to track our specific patches
			foundSchedulerName := false
			foundMaxCarbon := false
			foundEnforcement := false

			for _, patch := range resp.Patches {
				if patch.Operation == "add" && patch.Path == "/spec/schedulerName" && patch.Value == "susk8s-scheduler" {
					foundSchedulerName = true
				}

				// Because the pod started with nil annotations, the patch creates the whole map
				if patch.Operation == "add" && patch.Path == "/metadata/annotations" {
					if valMap, ok := patch.Value.(map[string]interface{}); ok {
						if valMap["susk8s.io/max-carbon"] == "300" {
							foundMaxCarbon = true
						}
						if valMap["susk8s.io/enforcement"] == "hard" {
							foundEnforcement = true
						}
					}
				}
			}

			Expect(foundSchedulerName).To(BeTrue(), "Failed to dynamically route to susk8s-scheduler")
			Expect(foundMaxCarbon).To(BeTrue(), "Failed to find the max-carbon limit injection")
			Expect(foundEnforcement).To(BeTrue(), "Failed to find the enforcement mode injection")
		})

		It("Should completely ignore the Pod IF it does not match any WorkloadPolicy", func() {
			ctx := context.Background()

			// Create a fake Pod without the matching target label
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standard-test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": "legacy-workload", // Does not match mock policy
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}

			rawPod, err := json.Marshal(pod)
			Expect(err).NotTo(HaveOccurred())

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: rawPod},
				},
			}

			resp := mutator.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue(), "Webhook should allow the request")
			Expect(resp.Patches).To(BeEmpty(), "Webhook should safely ignore standard pods")
		})

		It("Should return a 400 Bad Request if the API server sends invalid JSON", func() {
			ctx := context.Background()

			// Create broken JSON
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: []byte(`{ "broken": "json", missing_quotes }`)},
				},
			}
			resp := mutator.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse(), "Webhook should reject unparseable requests")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Code).To(Equal(int32(400))) // HTTP 400 Bad Request
		})
	})
})

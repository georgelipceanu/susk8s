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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("CarbonInfo Controller", func() {
	const (
		CarbonInfoName      = "test-carboninfo"
		CarbonInfoNamespace = "default"
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
	)

	Context("When updating CarbonInfo Status", func() {
		It("Should correctly apply the OverrideIntensity without calling external APIs", func() {
			ctx := context.Background()

			By("Creating a new CarbonInfo instance with an override")
			carbonInfo := &sustainabilityv1alpha1.CarbonInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CarbonInfoName,
					Namespace: CarbonInfoNamespace,
				},
				Spec: sustainabilityv1alpha1.CarbonInfoSpec{
					Region:            "GB",
					Provider:          "electricitymaps",
					PollSeconds:       60,
					OverrideIntensity: 850,
				},
			}

			Expect(k8sClient.Create(ctx, carbonInfo)).Should(Succeed())
			lookupKey := types.NamespacedName{Name: CarbonInfoName, Namespace: CarbonInfoNamespace}

			By("Manually triggering the Reconciler logic")
			reconciler := &CarbonInfoReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Fire the Reconcile function manually just like Kubernetes would
			req := reconcile.Request{NamespacedName: lookupKey}
			_, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())

			By("Fetching the updated resource")
			createdCarbonInfo := &sustainabilityv1alpha1.CarbonInfo{}
			Expect(k8sClient.Get(ctx, lookupKey, createdCarbonInfo)).Should(Succeed())

			By("Verifying the Status was updated")
			Expect(createdCarbonInfo.Status.CurrentIntensity).Should(BeEquivalentTo(850))

			Expect(k8sClient.Delete(ctx, carbonInfo)).Should(Succeed())
		})
	})

	Context("When the API fails and a Static Fallback is provided", func() {
		It("Should use StaticIntensity and annotate matching nodes", func() {
			ctx := context.Background()

			By("1. Creating a fake Node in the GB region")
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gb-test-node",
					Labels: map[string]string{
						"topology.kubernetes.io/region": "FAKE-REGION-404",
					},
				},
				Spec: corev1.NodeSpec{},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			By("Creating CarbonInfo pointing to real API (which will reject the sandbox token)")
			carbonInfo := &sustainabilityv1alpha1.CarbonInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fallback-carboninfo",
					Namespace: CarbonInfoNamespace,
				},
				Spec: sustainabilityv1alpha1.CarbonInfoSpec{
					Region:          "FAKE-REGION-404",
					Provider:        "electricitymaps",
					StaticIntensity: 400, // fallback value
				},
			}
			Expect(k8sClient.Create(ctx, carbonInfo)).Should(Succeed())

			By("Manually triggering the Reconciler")
			reconciler := &CarbonInfoReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "fallback-carboninfo", Namespace: CarbonInfoNamespace}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying CarbonInfo gracefully fell back to StaticIntensity")
			updatedCarbonInfo := &sustainabilityv1alpha1.CarbonInfo{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fallback-carboninfo", Namespace: CarbonInfoNamespace}, updatedCarbonInfo)).Should(Succeed())
			Expect(updatedCarbonInfo.Status.CurrentIntensity).Should(BeEquivalentTo(400))

			By("Verifying the Node received the live carbon annotation")
			updatedNode := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "gb-test-node"}, updatedNode)).Should(Succeed())
			Expect(updatedNode.Annotations["susk8s.io/carbon-intensity"]).Should(Equal("400"))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, carbonInfo)).Should(Succeed())
		})
	})
	Context("When the CarbonInfo resource does not exist", func() {
		It("Should ignore not found errors", func() {
			ctx := context.Background()

			reconciler := &CarbonInfoReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "does-not-exist",
					Namespace: CarbonInfoNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func makeReschedulingPolicy(name, namespace string, enabled bool, enforcement string, maxCarbon int32) *unstructured.Unstructured {
	mode := enforcement
	if mode == "" {
		mode = "soft"
	}

	policy := &unstructured.Unstructured{}
	policy.SetAPIVersion("sustainability.susk8s/v1alpha1")
	policy.SetKind("WorkloadPolicy")
	policy.SetName(name)
	policy.SetNamespace(namespace)
	policy.Object["spec"] = map[string]interface{}{
		"target": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"susk8s.io/tier": "test",
			},
		},
		"maxCarbonIntensity": int64(maxCarbon),
		"enforcement":        enforcement,
		"schedulerHints": map[string]interface{}{
			"carbonWeight":      int64(80),
			"utilisationWeight": int64(20),
		},
		"reschedule": map[string]interface{}{
			"enabled":           enabled,
			"mode":              mode,
			"cooldownSeconds":   int64(30),
			"evictionRateLimit": int64(5),
		},
	}
	return policy
}

func expectPodEvicted(ctx context.Context, namespace, name string) {
	Eventually(func(g Gomega) bool {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pod)
		if errors.IsNotFound(err) {
			return true
		}
		g.Expect(err).NotTo(HaveOccurred())
		return pod.DeletionTimestamp != nil
	}).Should(BeTrue())
}

func cleanupObject(ctx context.Context, obj client.Object) {
	_ = k8sClient.Delete(ctx, obj)
}

var _ = Describe("Rescheduling Controller", func() {
	Context("When enforcing Dynamic Rescheduling Modes", func() {

		It("Should aggressively evict pods in HARD mode to chase greener nodes (Proactive)", func() {
			ctx := context.Background()
			namespace := "default"
			policyName := "hard-proactive-policy"
			nodeName := "okay-node"
			betterNodeName := "super-green-node"
			podName := "hard-pod"
			maxCarbon := int32(300)

			// Create a node that is currently okay (no absolute violation)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        nodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "250"},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, node)

			// Create a greener node
			betterNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        betterNodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "50"},
				},
			}
			Expect(k8sClient.Create(ctx, betterNode)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, betterNode)

			// Create Pod on the okay node
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Annotations: map[string]string{
						"susk8s.io/max-carbon":  "300",
						"susk8s.io/enforcement": "hard", // HARD MODE
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, pod)

			// Create Policy
			policy := makeReschedulingPolicy(policyName, namespace, true, "hard", maxCarbon)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, policy)

			// Trigger Reconciler
			reconciler := &ReschedulingReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: policyName, Namespace: namespace}})
			Expect(err).NotTo(HaveOccurred())

			// Verify Eviction happened because the 50g node existed.
			expectPodEvicted(ctx, namespace, podName)

		})

		It("Should NOT proactively evict in SOFT mode if absolute limits are respected (Reactive Only)", func() {
			ctx := context.Background()
			namespace := "default"
			policyName := "soft-reactive-policy"
			nodeName := "okay-node-soft"
			betterNodeName := "super-green-node-soft"
			podName := "soft-pod-no-evict"
			maxCarbon := int32(300)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        nodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "250"},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, node)

			betterNode := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        betterNodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "50"},
				},
			}
			Expect(k8sClient.Create(ctx, betterNode)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, betterNode)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Annotations: map[string]string{
						"susk8s.io/max-carbon":  "300",
						"susk8s.io/enforcement": "soft", // SOFT MODE
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, pod)

			policy := makeReschedulingPolicy(policyName, namespace, true, "soft", maxCarbon)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, policy)

			reconciler := &ReschedulingReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: policyName, Namespace: namespace}})
			Expect(err).NotTo(HaveOccurred())

			// Verify NO Eviction because soft mode ignores greener nodes if the limit isn't broken
			survivingPod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, survivingPod)).Should(Succeed())
			Expect(survivingPod.DeletionTimestamp).To(BeNil())

		})

		It("Should evict in SOFT mode when an absolute limit violation occurs", func() {
			ctx := context.Background()
			namespace := "default"
			policyName := "soft-violation-policy"
			nodeName := "dirty-node-soft"
			podName := "soft-pod-evict"
			maxCarbon := int32(300)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        nodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "500"}, // 500 > 300
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, node)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Annotations: map[string]string{
						"susk8s.io/max-carbon":  "300",
						"susk8s.io/enforcement": "soft",
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, pod)

			policy := makeReschedulingPolicy(policyName, namespace, true, "soft", maxCarbon)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, policy)

			reconciler := &ReschedulingReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: policyName, Namespace: namespace}})
			Expect(err).NotTo(HaveOccurred())

			// Verify Eviction happened because the 300g absolute limit was broken.
			expectPodEvicted(ctx, namespace, podName)

		})

		It("Should completely ignore violations if Reschedule is DISABLED", func() {
			ctx := context.Background()
			namespace := "default"
			policyName := "disabled-policy"
			nodeName := "dirty-node-disabled"
			podName := "disabled-pod"
			maxCarbon := int32(300)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        nodeName,
					Annotations: map[string]string{"susk8s.io/carbon-intensity": "999"},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, node)

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Annotations: map[string]string{
						"susk8s.io/max-carbon":  "300",
						"susk8s.io/enforcement": "hard",
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, pod)

			policy := makeReschedulingPolicy(policyName, namespace, false, "hard", maxCarbon) // DISABLED
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())
			DeferCleanup(cleanupObject, ctx, policy)

			reconciler := &ReschedulingReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: policyName, Namespace: namespace}})
			Expect(err).NotTo(HaveOccurred())

			survivingPod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, survivingPod)).Should(Succeed())
			Expect(survivingPod.DeletionTimestamp).To(BeNil())

		})
	})
})

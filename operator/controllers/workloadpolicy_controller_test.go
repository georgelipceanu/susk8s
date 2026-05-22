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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

var _ = Describe("WorkloadPolicy Controller", func() {
	const (
		PolicyName      = "test-policy"
		PolicyNamespace = "default"
	)

	Context("When reconciling a WorkloadPolicy", func() {
		It("Should passively observe matching deployments and accurately update the status count", func() {
			ctx := context.Background()
			replicas := int32(1)

			By("Creating a Deployment that MATCHES the policy")
			matchingDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-dep",
					Namespace: PolicyNamespace,
					Labels:    map[string]string{"app": "green-workload"}, // This matches
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "green-workload"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "green-workload"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, matchingDep)).Should(Succeed())

			By("Creating a Deployment that DOES NOT match the policy")
			ignoredDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ignored-dep",
					Namespace: PolicyNamespace,
					Labels:    map[string]string{"app": "legacy-workload"}, // This does not match
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "legacy-workload"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "legacy-workload"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ignoredDep)).Should(Succeed())

			By("Creating the WorkloadPolicy")
			policy := &sustainabilityv1alpha1.WorkloadPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PolicyName,
					Namespace: PolicyNamespace,
				},
				Spec: sustainabilityv1alpha1.WorkloadPolicySpec{
					Enforcement: "hard",
					Target: sustainabilityv1alpha1.WorkloadTarget{
						MatchLabels: map[string]string{"app": "green-workload"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())

			By("Manually triggering the Reconciler")
			reconciler := &WorkloadPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: PolicyName, Namespace: PolicyNamespace}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			By("Verifying the Policy Status counted exactly 1 match")
			updatedPolicy := &sustainabilityv1alpha1.WorkloadPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: PolicyName, Namespace: PolicyNamespace}, updatedPolicy)).Should(Succeed())
			Expect(updatedPolicy.Status.Enforced).Should(BeTrue())
			Expect(updatedPolicy.Status.MatchedWorkloads).Should(BeEquivalentTo(1))

			By("Cleaning up the test cluster")
			Expect(k8sClient.Delete(ctx, matchingDep)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, ignoredDep)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, policy)).Should(Succeed())
		})

		It("Should safely ignore all deployments if the policy has empty MatchLabels", func() {
			ctx := context.Background()

			By("Creating a WorkloadPolicy with an EMPTY Target")
			emptyPolicy := &sustainabilityv1alpha1.WorkloadPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-policy",
					Namespace: PolicyNamespace,
				},
				Spec: sustainabilityv1alpha1.WorkloadPolicySpec{
					Enforcement: "soft",
					// MatchLabels is intentionally left empty to trigger your safety check
					Target: sustainabilityv1alpha1.WorkloadTarget{},
				},
			}
			Expect(k8sClient.Create(ctx, emptyPolicy)).Should(Succeed())

			By("Triggering the Reconciler")
			reconciler := &WorkloadPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "empty-policy", Namespace: PolicyNamespace}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Reconciler caught the empty labels and matched 0 workloads")
			updatedPolicy := &sustainabilityv1alpha1.WorkloadPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "empty-policy", Namespace: PolicyNamespace}, updatedPolicy)).Should(Succeed())
			Expect(updatedPolicy.Status.MatchedWorkloads).Should(BeEquivalentTo(0))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, emptyPolicy)).Should(Succeed())
		})
	})
})

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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("CarbonInfo Controller", func() {
	// Define common variables for the tests
	const (
		CarbonInfoName      = "test-carboninfo"
		CarbonInfoNamespace = "default"
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
	)

	Context("When updating CarbonInfo Status", func() {
		It("Should correctly apply the OverrideIntensity without calling external APIs", func() {
			ctx := context.Background()

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

			reconciler := &CarbonInfoReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			req := reconcile.Request{NamespacedName: lookupKey}
			_, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())

			createdCarbonInfo := &sustainabilityv1alpha1.CarbonInfo{}
			Expect(k8sClient.Get(ctx, lookupKey, createdCarbonInfo)).Should(Succeed())

			Expect(createdCarbonInfo.Status.CurrentIntensity).Should(BeEquivalentTo(850))

			Expect(k8sClient.Delete(ctx, carbonInfo)).Should(Succeed())
		})
	})
})

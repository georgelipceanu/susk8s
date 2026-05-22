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
)

var _ = Describe("EmissionReport Controller", func() {
	// Define constants so we don't have magic strings floating around
	const (
		ReportName      = "test-audit-report"
		ReportNamespace = "default"
		timeout         = time.Second * 10
		interval        = time.Millisecond * 250
	)

	Context("When creating a new EmissionReport", func() {
		It("Should successfully create the resource and trigger the Reconciler", func() {
			ctx := context.Background()

			By("Parsing the timestamps into metav1.Time")
			// Parse the RFC3339 strings into standard Go time.Time objects
			startTime, _ := time.Parse(time.RFC3339, "2026-05-01T00:00:00Z")
			endTime, _ := time.Parse(time.RFC3339, "2026-05-02T00:00:00Z")

			By("Building a valid EmissionReport CR")
			report := &sustainabilityv1alpha1.EmissionReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ReportName,
					Namespace: ReportNamespace,
				},
				Spec: sustainabilityv1alpha1.EmissionReportSpec{
					Scope:  "namespace",
					Region: "IE",
					// Wrap the parsed times in Kubernetes' metav1.Time wrapper!
					From: metav1.Time{Time: startTime},
					To:   metav1.Time{Time: endTime},
				},
			}

			By("Sending the creation request to the test cluster")
			Expect(k8sClient.Create(ctx, report)).Should(Succeed())

			By("Fetching the created resource to ensure it exists")
			lookupKey := types.NamespacedName{Name: ReportName, Namespace: ReportNamespace}
			createdReport := &sustainabilityv1alpha1.EmissionReport{}

			Eventually(func() error {
				return k8sClient.Get(ctx, lookupKey, createdReport)
			}, timeout, interval).Should(Succeed())

			// Ensure the data didn't get mangled
			Expect(createdReport.Spec.Region).Should(Equal("IE"))
			Expect(createdReport.Spec.Scope).Should(Equal("namespace"))

			By("Cleaning up the test cluster")
			Expect(k8sClient.Delete(ctx, report)).Should(Succeed())
		})
	})
})

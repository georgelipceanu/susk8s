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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	sustainabilityv1alpha1 "susk8s/operator/api/v1alpha1"
)

type MockPromAPI struct {
	promv1.API
	ForceError          bool
	MockTotalResult     model.Value
	MockBreakdownResult model.Value
}

func (m *MockPromAPI) Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if m.ForceError {
		return nil, nil, fmt.Errorf("mock error forcing fallback")
	}

	if strings.Contains(query, "sum by") {
		return m.MockBreakdownResult, nil, nil
	}

	return m.MockTotalResult, nil, nil
}

var _ = Describe("EmissionReport Controller", func() {
	const ReportNamespace = "default"

	createCarbonInfo := func(ctx context.Context, name, region string) *sustainabilityv1alpha1.CarbonInfo {
		now := metav1.Now()
		carbonInfo := &sustainabilityv1alpha1.CarbonInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ReportNamespace,
			},
			Spec: sustainabilityv1alpha1.CarbonInfoSpec{
				Provider:    "mock",
				Region:      region,
				PollSeconds: 60,
			},
		}

		Expect(k8sClient.Create(ctx, carbonInfo)).Should(Succeed())
		carbonInfo.Status.CurrentIntensity = 100
		carbonInfo.Status.LastUpdated = &now
		Expect(k8sClient.Status().Update(ctx, carbonInfo)).Should(Succeed())

		return carbonInfo
	}

	Context("When reconciling an EmissionReport", func() {
		It("Should calculate zero emissions when Prometheus fails and the controller fallback is zero", func() {
			ctx := context.Background()

			carbonInfo := createCarbonInfo(ctx, "ie-carbon-info-fallback", "IE")

			startTime, _ := time.Parse(time.RFC3339, "2026-05-01T00:00:00Z")
			endTime, _ := time.Parse(time.RFC3339, "2026-05-02T00:00:00Z")

			report := &sustainabilityv1alpha1.EmissionReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fallback-telemetry-report",
					Namespace: ReportNamespace,
				},
				Spec: sustainabilityv1alpha1.EmissionReportSpec{
					Scope:  "namespace",
					Region: "IE",
					From:   metav1.Time{Time: startTime},
					To:     metav1.Time{Time: endTime},
				},
			}
			Expect(k8sClient.Create(ctx, report)).Should(Succeed())

			reconciler := &EmissionReportReconciler{
				Client:  k8sClient,
				Scheme:  k8sClient.Scheme(),
				PromAPI: &MockPromAPI{ForceError: true},
			}

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: report.Name, Namespace: report.Namespace}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			updatedReport := &sustainabilityv1alpha1.EmissionReport{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: report.Name, Namespace: report.Namespace}, updatedReport)).Should(Succeed())

			Expect(updatedReport.Status.Conditions).To(HaveLen(1))
			Expect(updatedReport.Status.Conditions[0].Type).To(Equal("Finished"))
			Expect(updatedReport.Status.Totals.EnergyKWh).Should(BeNumerically("==", 0.0))
			Expect(updatedReport.Status.Totals.EmissionsGCO2).Should(BeNumerically("==", 0.0))

			Expect(k8sClient.Delete(ctx, report)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, carbonInfo)).Should(Succeed())
		})

		It("Should safely requeue if no CarbonInfo exists for the requested region", func() {
			ctx := context.Background()

			report := &sustainabilityv1alpha1.EmissionReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-region-report",
					Namespace: ReportNamespace,
				},
				Spec: sustainabilityv1alpha1.EmissionReportSpec{
					Scope:  "namespace",
					Region: "US",
					From:   metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
					To:     metav1.Time{Time: time.Now()},
				},
			}
			Expect(k8sClient.Create(ctx, report)).Should(Succeed())

			reconciler := &EmissionReportReconciler{
				Client:  k8sClient,
				Scheme:  k8sClient.Scheme(),
				PromAPI: &MockPromAPI{ForceError: true},
			}

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: report.Name, Namespace: report.Namespace}}
			res, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(Equal(10 * time.Second))

			Expect(k8sClient.Delete(ctx, report)).Should(Succeed())
		})

		It("Should dynamically calculate breakdown map for advanced Deployment Scopes", func() {
			ctx := context.Background()

			carbonInfo := createCarbonInfo(ctx, "ie-carbon-info-deployment", "IE")

			report := &sustainabilityv1alpha1.EmissionReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-breakdown-report",
					Namespace: ReportNamespace,
				},
				Spec: sustainabilityv1alpha1.EmissionReportSpec{
					Scope:  "deployment",
					Region: "IE",
					From:   metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
					To:     metav1.Time{Time: time.Now()},
				},
			}
			Expect(k8sClient.Create(ctx, report)).Should(Succeed())

			mockTotal := model.Vector{
				&model.Sample{Value: 10800000.0}, // Total: 3 kWh
			}

			mockBreakdown := model.Vector{
				&model.Sample{
					Metric: model.Metric{model.LabelName("deployment"): model.LabelValue("frontend-web")},
					Value:  3600000.0, // 1 kWh
				},
				&model.Sample{
					Metric: model.Metric{model.LabelName("deployment"): model.LabelValue("backend-api")},
					Value:  7200000.0, // 2 kWh
				},
			}

			reconciler := &EmissionReportReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				PromAPI: &MockPromAPI{
					ForceError:          false,
					MockTotalResult:     mockTotal,
					MockBreakdownResult: mockBreakdown,
				},
			}

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: report.Name, Namespace: report.Namespace}}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			updatedReport := &sustainabilityv1alpha1.EmissionReport{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: report.Name, Namespace: report.Namespace}, updatedReport)).Should(Succeed())

			Expect(updatedReport.Status.Breakdown).To(HaveLen(2))
			Expect(updatedReport.Status.Breakdown).To(HaveKey("frontend-web"))
			Expect(updatedReport.Status.Breakdown).To(HaveKey("backend-api"))

			Expect(updatedReport.Status.Breakdown["frontend-web"].EnergyKWh).To(BeNumerically("==", 1.0))
			Expect(updatedReport.Status.Breakdown["frontend-web"].EmissionsGCO2).To(BeNumerically("==", 100.0))

			Expect(updatedReport.Status.Breakdown["backend-api"].EnergyKWh).To(BeNumerically("==", 2.0))
			Expect(updatedReport.Status.Breakdown["backend-api"].EmissionsGCO2).To(BeNumerically("==", 200.0))

			Expect(k8sClient.Delete(ctx, report)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, carbonInfo)).Should(Succeed())
		})
	})
})

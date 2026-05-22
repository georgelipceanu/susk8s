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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReportTotals represents the calculated sustainability metrics.
type ReportTotals struct {
	// EnergyKWh is the estimated energy consumption for the report period.
	// +optional
	EnergyKWh float64 `json:"energyKWh"`

	// EmissionsGCO2 is the estimated carbon footprint in grams of CO2.
	// +optional
	EmissionsGCO2 float64 `json:"emissionsGCO2"`
}

// EmissionReportSpec defines the parameters for generating a sustainability audit.
type EmissionReportSpec struct {
	// Scope defines the level of data aggregation.
	// Valid values are: "namespace", "workload", "deployment" or "node".
	// +kubebuilder:validation:Enum=namespace;workload;node;deployment
	// +required
	Scope string `json:"scope"`

	// Region links this report to a specific grid carbon intensity.
	// +required
	Region string `json:"region"`

	// From is the start timestamp of the reporting window.
	// +required
	From metav1.Time `json:"from"`

	// To is the end timestamp of the reporting window.
	// +required
	To metav1.Time `json:"to"`
}

// EmissionReportStatus defines the observed results of the sustainability audit.
type EmissionReportStatus struct {
	// Totals contains the overall cluster-wide energy and emissions for the window.
	// +optional
	Totals ReportTotals `json:"totals,omitempty"`

	// Breakdown provides a detailed view per individual entity (e.g., per namespace).
	// +optional
	Breakdown map[string]ReportTotals `json:"breakdown,omitempty"`

	// Conditions represent the current state of report generation (e.g., Finished, DataPartial).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=emrep
// +kubebuilder:printcolumn:name="Scope",type=string,JSONPath=`.spec.scope`
// +kubebuilder:printcolumn:name="Energy_kWh",type=number,JSONPath=`.status.totals.energyKWh`
// +kubebuilder:printcolumn:name="Emissions_gCO2",type=number,JSONPath=`.status.totals.emissionsGCO2`

// EmissionReport is the Schema for the emissionreports API.
// It materialises auditable energy and emission summaries based on Kepler and Prometheus metrics.
type EmissionReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmissionReportSpec   `json:"spec,omitempty"`
	Status EmissionReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EmissionReportList contains a list of EmissionReport.
type EmissionReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmissionReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EmissionReport{}, &EmissionReportList{})
}

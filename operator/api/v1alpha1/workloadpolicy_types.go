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

// WorkloadTarget selects the workloads a policy applies to.
type WorkloadTarget struct {
	// NamespaceSelector optionally restricts to namespaces with these labels.
	// If empty, all namespaces are considered.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// MatchLabels selects workloads by labels.
	// This maps directly to Pod.metadata.labels for matching.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// SchedulerHints contains per-workload preferences for the scheduler plugin.
type SchedulerHints struct {
	// UtilisationWeight is an integer 0–100.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	UtilisationWeight *int32 `json:"utilisationWeight,omitempty"`

	// CarbonWeight is an integer 0–100.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CarbonWeight *int32 `json:"carbonWeight,omitempty"`
}

// ReschedulePolicy defines the rules for post-placement correction.
type ReschedulePolicy struct {
	// Enabled turns on/off the rescheduling logic for this policy.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Mode defines if violations result in a "soft" warning (event) or a "hard" eviction.
	// +kubebuilder:validation:Enum=soft;hard
	// +kubebuilder:default=soft
	Mode string `json:"mode"`

	// CooldownSeconds defines the minimum time between rescheduling actions to prevent churn.
	// +kubebuilder:validation:Minimum=0
	// +optional
	CooldownSeconds int32 `json:"cooldownSeconds,omitempty"`

	// MinImprovement is the minimum % of sustainability improvement required to trigger eviction.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinImprovement int32 `json:"minImprovement,omitempty"`

	// EvictionRateLimit restricts the number of pods evicted per reconciliation cycle.
	// +kubebuilder:validation:Minimum=0
	// +optional
	EvictionRateLimit int32 `json:"evictionRateLimit,omitempty"`
}

// WorkloadPolicySpec describes the sustainability preferences for a set of workloads.
type WorkloadPolicySpec struct {
	// Target defines which workloads are governed by this policy.
	Target WorkloadTarget `json:"target"`

	// MaxCarbonIntensity is an optional upper bound (gCO2/kWh) for eligible nodes.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxCarbonIntensity *int32 `json:"maxCarbonIntensity,omitempty"`

	// SchedulerHints define how the CarbonBinPack plugin should weigh utilisation vs carbon.
	// +optional
	SchedulerHints *SchedulerHints `json:"schedulerHints,omitempty"`

	// Enforcement controls whether the policy is a soft preference or a hard requirement.
	// Allowed values: "soft", "hard".
	// +kubebuilder:validation:Enum=soft;hard
	// +kubebuilder:default=soft
	Enforcement string `json:"enforcement,omitempty"`

	// Reschedule defines the policy for eviction-based rescheduling when conditions change.
	// +optional
	Reschedule *ReschedulePolicy `json:"reschedule,omitempty"`
}

// WorkloadPolicyStatus reports how effectively the policy is applied.
type WorkloadPolicyStatus struct {
	// Enforced indicates whether hints/constraints have been applied successfully.
	// +optional
	Enforced bool `json:"enforced,omitempty"`

	// MatchedWorkloads is the number of workloads currently targeted by this policy.
	// +optional
	MatchedWorkloads int32 `json:"matchedWorkloads,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// WorkloadPolicy declares carbon-aware scheduling preferences for a set of workloads.
type WorkloadPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadPolicySpec   `json:"spec,omitempty"`
	Status WorkloadPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// WorkloadPolicyList contains a list of WorkloadPolicy.
type WorkloadPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadPolicy{}, &WorkloadPolicyList{})
}

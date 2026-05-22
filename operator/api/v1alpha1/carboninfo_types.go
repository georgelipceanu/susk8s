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

// CarbonInfoSpec defines the desired state of CarbonInfo.
// It configures how and where the system should fetch carbon intensity data.
type CarbonInfoSpec struct {
	// Provider is the data source.
	// +kubebuilder:validation:Required
	Provider string `json:"provider"`

	// Region is the provider-specific geographic key for the grid data.
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// PollSeconds defines the refresh cadence for fetching live intensity data.
	// +kubebuilder:default=300
	// +optional
	PollSeconds int32 `json:"pollSeconds,omitempty"`

	// StaticIntensity provides a fallback or fixed gCO2/kWh value for static mode.
	// +optional
	StaticIntensity int32 `json:"staticIntensity,omitempty"`

	// +optional
	OverrideIntensity int32 `json:"overrideIntensity,omitempty"`
}

// CarbonInfoStatus defines the observed state of CarbonInfo.
type CarbonInfoStatus struct {
	// CurrentIntensity is the latest fetched grid carbon intensity in gCO2/kWh.
	// +optional
	CurrentIntensity int32 `json:"currentIntensity,omitempty"`

	// LastUpdated is the timestamp of the last successful data retrieval from the provider.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// Conditions represent the current state of the carbon signal (e.g., Ready, Error).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=carbon
// +kubebuilder:printcolumn:name="Intensity",type=integer,JSONPath=`.status.currentIntensity`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="Last_Update",type=date,JSONPath=`.status.lastUpdated`

// CarbonInfo is the Schema for the carboninfoes API.
// It represents regional or node-group carbon intensity as a live signal for the cluster.
type CarbonInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CarbonInfoSpec   `json:"spec"`
	Status CarbonInfoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CarbonInfoList contains a list of CarbonInfo
type CarbonInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CarbonInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CarbonInfo{}, &CarbonInfoList{})
}

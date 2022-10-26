/*
Copyright 2022.

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

// Ref assumes that the namespace is the same as whatever the component has that is looking for this ref.
type Ref struct {
	Name string `json:"name"`
}

// Verify contains a ref to the key used for verification.
type Verify struct {
	// Name of the signature.
	Name string `json:"name"`
	// Key which is used for verification.
	Key Ref `json:"key"`
}

// Signature defines the details of a signature to use for verification.
type Signature struct {
	Verify Verify `json:"signature"`
}

// OCIRepository defines details for a repository, such as access keys and the url.
type OCIRepository struct {
	URL       string `json:"url"`
	SecretRef Ref    `json:"secretRef"`
}

// ComponentSubscriptionSpec defines the desired state of ComponentSubscription
type ComponentSubscriptionSpec struct {
	Interval    string        `json:"interval"`
	Source      OCIRepository `json:"source"`
	Destination OCIRepository `json:"destination"`
	Component   string        `json:"component"`
	// +optional
	Semver string      `json:"semver,omitempty"`
	Verify []Signature `json:"verify"`
}

// ComponentSubscriptionStatus defines the observed state of ComponentSubscription
type ComponentSubscriptionStatus struct {
	ReplicatedVersion string `json:"replicatedVersion"`
	LatestVersion     string `json:"latestVersion"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ComponentSubscription is the Schema for the componentsubscriptions API
type ComponentSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSubscriptionSpec   `json:"spec,omitempty"`
	Status ComponentSubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentSubscriptionList contains a list of ComponentSubscription
type ComponentSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentSubscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentSubscription{}, &ComponentSubscriptionList{})
}

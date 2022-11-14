// Copyright 2022.
// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

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

// SecretRef clearly denotes that the requested option is a Secret.
type SecretRef struct {
	SecretRef Ref `json:"secretRef"`
}

// Signature defines the details of a signature to use for verification.
type Signature struct {
	// Name of the signature.
	Name string `json:"name"`
	// Key which is used for verification.
	PublicKey SecretRef `json:"publicKey"`
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

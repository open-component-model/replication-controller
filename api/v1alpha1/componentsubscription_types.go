// Copyright 2022.
// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

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

// OCMRepository defines details for a repository, such as access keys and the url.
type OCMRepository struct {
	URL       string `json:"url"`
	SecretRef *Ref   `json:"secretRef,omitempty"`
}

// ComponentSubscriptionSpec defines the desired state of ComponentSubscription
type ComponentSubscriptionSpec struct {
	// Interval is the reconciliation interval, i.e. at what interval shall a reconciliation happen.
	// This is used to requeue objects for reconciliation in case of success as well as already reconciling objects.
	// +required
	Interval metav1.Duration `json:"interval"`

	Source      OCMRepository `json:"source"`
	Destination OCMRepository `json:"destination"`
	Component   string        `json:"component"`
	//+optional
	Semver string      `json:"semver,omitempty"`
	Verify []Signature `json:"verify,omitempty"`
}

// ComponentSubscriptionStatus defines the observed state of ComponentSubscription
type ComponentSubscriptionStatus struct {
	// LatestVersion defines the version that was last reconciled successfully.
	LatestVersion string `json:"latestVersion"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+optional
	ReplicatedVersion string `json:"replicatedVersion,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *ComponentSubscription) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *ComponentSubscription) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// GetRequeueAfter returns the duration after which the ComponentVersion must be
// reconciled again.
func (in ComponentSubscription) GetRequeueAfter() time.Duration {
	return in.Spec.Interval.Duration
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

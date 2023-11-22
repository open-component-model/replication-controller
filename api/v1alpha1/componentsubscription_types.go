// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComponentSubscriptionSpec defines the desired state of ComponentSubscription. It specifies
// the parameters that the replication controller will use to replicate a desired Component from
// a source OCM repository to a destination OCM repository.
type ComponentSubscriptionSpec struct {
	// Component specifies the name of the Component that should be replicated.
	// +required
	Component string `json:"component"`

	// Semver specifies an optional semver constraint that is used to evaluate the component
	// versions that should be replicated.
	//+optional
	Semver string `json:"semver,omitempty"`

	// Source holds the OCM Repository details for the replication source.
	// +required
	Source OCMRepository `json:"source"`

	// Destination holds the destination or target OCM Repository details. The ComponentVersion
	// will be transferred into this repository.
	// +optional
	Destination *OCMRepository `json:"destination,omitempty"`

	// Interval is the reconciliation interval, i.e. at what interval shall a reconciliation happen.
	// This is used to requeue objects for reconciliation in case of success as well as already reconciling objects.
	// +required
	Interval metav1.Duration `json:"interval"`

	// ServiceAccountName can be used to configure access to both destination and source repositories.
	// If service account is defined, it's usually redundant to define access to either source or destination, but
	// it is still allowed to do so.
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Verify specifies a list signatures that must be verified before a ComponentVersion
	// is replicated.
	// +optional
	Verify []Signature `json:"verify,omitempty"`
}

// Signature defines the details of a signature to use for verification.
type Signature struct {
	// Name specifies the name of the signature. An OCM component may have multiple
	// signatures.
	Name string `json:"name"`

	// PublicKey provides a reference to a Kubernetes Secret that contains a public key
	// which will be used to validate the named signature.
	PublicKey SecretRef `json:"publicKey"`
}

// SecretRef clearly denotes that the requested option is a Secret.
type SecretRef struct {
	SecretRef meta.LocalObjectReference `json:"secretRef"`
}

// OCMRepository specifies access details for an OCI based OCM Repository.
type OCMRepository struct {
	// URL specifies the URL of the OCI registry.
	// +required
	URL string `json:"url"`

	// SecretRef specifies the credentials used to access the OCI registry.
	// +optional
	SecretRef *meta.LocalObjectReference `json:"secretRef,omitempty"`
}

// ComponentSubscriptionStatus defines the observed state of ComponentSubscription.
type ComponentSubscriptionStatus struct {
	// LastAttemptedVersion defines the latest version encountered while checking component versions.
	// This might be different from last applied version which should be the latest applied/replicated version.
	// The difference might be caused because of semver constraint or failures during replication.
	//+optional
	LastAttemptedVersion string `json:"lastAttemptedVersion,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastAppliedVersion defines the final version that has been applied to the destination component version.
	//+optional
	LastAppliedVersion string `json:"lastAppliedVersion,omitempty"`

	// ReplicatedRepositoryURL defines the final location of the reconciled Component.
	//+optional
	ReplicatedRepositoryURL string `json:"replicatedRepositoryURL,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (in *ComponentSubscription) GetVID() map[string]string {
	vid := fmt.Sprintf("%s:%s", in.Status.LastAttemptedVersion, in.Status.LastAppliedVersion)
	metadata := make(map[string]string)
	metadata[GroupVersion.Group+"/component_subscription"] = vid

	return metadata
}

func (in *ComponentSubscription) SetObservedGeneration(v int64) {
	in.Status.ObservedGeneration = v
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

// Registry defines information about the location of a component.
type Registry struct {
	URL string `json:"url"`
}

// Component holds information about a reconciled component.
type Component struct {
	// Name specifies the component name.
	Name string `json:"name"`

	// Version specifies the component version.
	Version string `json:"version"`

	// Version specifies the component registry.
	Registry Registry `json:"registry"`
}

// GetComponentVersion returns a constructed component version with name, version and reconciled location.
func (in ComponentSubscription) GetComponentVersion() Component {
	return Component{
		Name:    in.Spec.Component,
		Version: in.Status.LastAppliedVersion,
		Registry: Registry{
			URL: in.Status.ReplicatedRepositoryURL,
		},
	}
}

//+kubebuilder:resource:shortName=coms
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ComponentSubscription is the Schema for the componentsubscriptions API.
type ComponentSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSubscriptionSpec   `json:"spec,omitempty"`
	Status ComponentSubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentSubscriptionList contains a list of ComponentSubscription.
type ComponentSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentSubscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentSubscription{}, &ComponentSubscriptionList{})
}

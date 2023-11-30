// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// AuthenticationFailedReason is used when we couldn't authenticate the OCM context.
	AuthenticationFailedReason = "AuthenticationFailed"

	// PullingLatestVersionFailedReason is used when we couldn't pull the latest version for a controller.
	PullingLatestVersionFailedReason = "PullingLatestVersionFailed"

	// SemverConversionFailedReason is used when we couldn't convert a version to semver.
	SemverConversionFailedReason = "SemverConversionFailed"

	// GetComponentDescriptorFailedReason is used when the component descriptor cannot be retrieved.
	GetComponentDescriptorFailedReason = "GetComponentDescriptorFailed"

	// TransferFailedReason is used when we fail to transfer a component.
	TransferFailedReason = "TransferFailed"

	// ComponentSigningFailedReason is used when we can't sign the component that will be transferred.
	ComponentSigningFailedReason = "ComponentSigningFailed"
)

// Copyright 2022.
// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// ConfiguringCredentialsFailedReason is used when we fail to configure credentials.
	ConfiguringCredentialsFailedReason = "ConfiguringCredentialsFailed"

	// PatchFailedReason is used when we couldn't patch an object.
	PatchFailedReason = "PatchFailed"

	// PullingLatestVersionFailedReason is used when we couldn't pull the latest version for a controller.
	PullingLatestVersionFailedReason = "PullingLatestVersionFailed"

	// SemverConversionFailedReason is used when we couldn't convert a version to semver.
	SemverConversionFailedReason = "SemverConversionFailed"

	// GetComponentDescriptorFailedReason is used when the component descriptor cannot be retrieved.
	GetComponentDescriptorFailedReason = "GetComponentDescriptorFailed"

	// RepositoryForSpecFailedReason is used when we fail to create a repository for a spec.
	RepositoryForSpecFailedReason = "RepositoryForSpecFailed"

	// VerificationProcessFailedReason is used when the verification process fails to verify a component.
	VerificationProcessFailedReason = "VerificationProcessFailed"

	// ConstructingHandlerFailedReason is used when we fail to create a transfer handler.
	ConstructingHandlerFailedReason = "ConstructingHandlerFailed"

	// TransferFailedReason is used when we fail to transfer a component.
	TransferFailedReason = "TransferFailed"
)

// Copyright 2022.
// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// ComponentVersionFetchFailedReason is used when we failed to retrieve a component version.
	ComponentVersionFetchFailedReason = "ComponentVersionFetchFailed"

	// ComponentDescriptorFetchFailedReason is used when we failed to retrieve a component descriptor.
	ComponentDescriptorFetchFailedReason = "ComponentDescriptorFetchFailed"

	// PipelineTemplateFetchFailedReason is used when we failed to retrieve the pipeline template.
	PipelineTemplateFetchFailedReason = "PipelineTemplateFetchFailed"

	// PipelineTemplateExecutionFailedReason is used when we failed to execute the pipeline template.
	PipelineTemplateExecutionFailedReason = "PipelineTemplateExecutionFailed"

	// InvalidPipelineTemplateDataReason is used when we can't unmarshal the template data.
	InvalidPipelineTemplateDataReason = "InvalidPipelineTemplateData"

	// SettingDefaultValuesFailedReason is used when we fail to set default values for Kind data.
	SettingDefaultValuesFailedReason = "SettingDefaultValuesFailed"

	// TemplateStepDependencyErrorReason is used when the template steps ordering if invalid and there is a missing
	// step that another step is depending on.
	TemplateStepDependencyErrorReason = "TemplateStepDependencyError"

	// TemplateDataParseFailedReason is used when we fail to parse template data.
	TemplateDataParseFailedReason = "TemplateDataParseFailed"

	// ImpersonatingClientFetchFailedReason is used when we can't create an impersonation client.
	ImpersonatingClientFetchFailedReason = "ImpersonatingClientFetchFailed"

	// ShouldReconcileCheckFailedReason is used when we couldn't determine if the controller should reconcile.
	ShouldReconcileCheckFailedReason = "ShouldReconcileCheckFailed"

	// PatchFailedReason is used when we couldn't patch an object.
	PatchFailedReason = "PatchFailed"

	// ApplyFailedReason is used when we couldn't apply an object.
	ApplyFailedReason = "ApplyFailed"

	// PruneFailedReason is used when we couldn't prune an object.
	PruneFailedReason = "PruneFailed"

	// AddingItemsToInventoryFailedReason is used when we couldn't add any items to the inventory.
	AddingItemsToInventoryFailedReason = "AddingItemsToInventoryFailed"

	// InventoryDiffFailedReason is used when we couldn't diff the inventory.
	InventoryDiffFailedReason = "InventoryDiffFailed"
)

// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	mh "github.com/open-component-model/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsComponent = "replication_controller"
)

func init() {
	metrics.Registry.MustRegister(SubscriptionsReconciledTotal, SubscriptionsReconcileFailed)
}

// SubscriptionsReconciledTotal counts the number times a subscription was reconciled.
var SubscriptionsReconciledTotal = mh.MustRegisterCounterVec(
	"ocm_system",
	metricsComponent,
	"subscription_reconciled_total",
	"Number of times a subscription was reconciled",
	"component", "version",
)

// SubscriptionsReconcileFailed counts the number times we failed to reconcile a subscription.
var SubscriptionsReconcileFailed = mh.MustRegisterCounterVec(
	"ocm_system",
	metricsComponent,
	"subscription_reconcile_failed",
	"Number of times a subscription failed to reconcile",
	"component",
)

// SubscriptionsTotalBytes counts the number of bytes reconciled for a specific component version.
var SubscriptionsTotalBytes = mh.MustRegisterCounterVec(
	"ocm_system",
	metricsComponent,
	"subscription_total_bytes",
	"Number of bytes reconciled for a specific version",
)

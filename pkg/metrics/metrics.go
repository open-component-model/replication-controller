// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	mh "github.com/open-component-model/pkg/metrics"
)

const (
	metricsComponent = "replication_controller"
)

// SubscriptionsReconciledTotal counts the number times a subscription was reconciled.
var SubscriptionsReconciledTotal = mh.MustRegisterCounterVec(
	"ocm_system",
	metricsComponent,
	"subscription_reconciled_total",
	"Number of times a subscription was reconciled",
)

// SubscriptionsReconcileFailed counts the number times we failed to reconcile a subscription
var SubscriptionsReconcileFailed = mh.MustRegisterCounter(
	"ocm_system",
	metricsComponent,
	"subscription_reconcile_failed",
	"Number of times a subscription failed to reconcile",
)

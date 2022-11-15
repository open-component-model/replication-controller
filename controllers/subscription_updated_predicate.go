// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
)

// SubscriptionUpdatedPredicate triggers an update event when status of a job changes.
type SubscriptionUpdatedPredicate struct {
	predicate.Funcs
}

func (SubscriptionUpdatedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldJob, ok := e.ObjectOld.(*v1alpha1.ComponentSubscription)
	if !ok {
		return false
	}

	newJob, ok := e.ObjectNew.(*v1alpha1.ComponentSubscription)
	if !ok {
		return false
	}

	if oldJob.Status.LatestVersion != newJob.Status.ReplicatedVersion {
		return true
	}

	return false
}

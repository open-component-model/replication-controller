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

package controllers

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
)

// ComponentSubscriptionReconciler reconciles a ComponentSubscription object
type ComponentSubscriptionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ComponentSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	subscription := &v1alpha1.ComponentSubscription{}
	if err := r.Get(ctx, req.NamespacedName, subscription); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return requeue(10 * time.Second), err
	}

	log = log.WithValues("subscription", subscription)
	log.Info("starting reconcile loop")

	if subscription.DeletionTimestamp != nil {
		log.Info("subscription is being deleted...")
		return ctrl.Result{}, nil
	}

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if subscription.Status.LatestVersion == subscription.Status.ReplicatedVersion &&
		(subscription.Status.LatestVersion != "" && subscription.Status.ReplicatedVersion != "") {
		log.Info("latest version and replicated version are a match and not empty, skipping reconciling...")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComponentSubscription{}).
		WithEventFilter(predicate.Or(SubscriptionUpdatedPredicate{})).
		Complete(r)
}

func requeue(seconds time.Duration) ctrl.Result {
	return ctrl.Result{
		RequeueAfter: seconds * time.Second,
	}
}

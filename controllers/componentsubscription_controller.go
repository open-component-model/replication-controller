// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/open-component-model/replication-controller/pkg/ocm"
)

// ComponentSubscriptionReconciler reconciles a ComponentSubscription object
type ComponentSubscriptionReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OCMClient ocm.Contract
}

//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ComponentSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)
	obj := &v1alpha1.ComponentSubscription{}
	if err = r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger = logger.WithValues("subscription", klog.KObj(obj))
	logger.V(4).Info("starting reconcile loop")

	if obj.DeletionTimestamp != nil {
		logger.Info("subscription is being deleted...")

		return
	}

	// The replication controller doesn't need a shouldReconcile, because it should always reconcile,
	// that is its purpose.
	patchHelper := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		if perr := patchHelper.Patch(ctx, obj); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	return r.reconcile(ctx, obj)
}

func (r *ComponentSubscriptionReconciler) reconcile(ctx context.Context, obj *v1alpha1.ComponentSubscription) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj)
	if err != nil {
		err := fmt.Errorf("failed to authenticate OCM context: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.AuthenticationFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	version, err := r.OCMClient.GetLatestSourceComponentVersion(ctx, octx, obj)
	if err != nil {
		err := fmt.Errorf("failed to get latest component version: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.PullingLatestVersionFailedReason, err.Error())

		// we don't want to fail but keep searching until it's there. But we do mark the subscription as failed.
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}
	logger.V(4).Info("got newest version from component", "version", version)

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if version == obj.Status.LastAppliedVersion {
		logger.Info("latest version and last applied version are a match and not empty")
		conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	latestSourceComponentVersion, err := semver.NewVersion(version)
	if err != nil {
		err := fmt.Errorf("failed to parse source component version: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.SemverConversionFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	lastAppliedOriginal := "0.0.0"
	if obj.Status.LastAppliedVersion != "" {
		lastAppliedOriginal = obj.Status.LastAppliedVersion
	}

	lastAppliedVersion, err := semver.NewVersion(lastAppliedOriginal)
	if err != nil {
		err := fmt.Errorf("failed to parse latest version: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.SemverConversionFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	logger.V(4).Info("latest applied version is", "version", lastAppliedVersion.Original())

	if latestSourceComponentVersion.LessThan(lastAppliedVersion) || latestSourceComponentVersion.Equal(lastAppliedVersion) {
		logger.Info("no new version found", "version", latestSourceComponentVersion.Original(), "latest", lastAppliedVersion.Original())
		conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// set latest version, this will be patched in the defer statement.
	obj.Status.LastAttemptedVersion = latestSourceComponentVersion.Original()

	sourceComponentVersion, err := r.OCMClient.GetComponentVersion(ctx, octx, obj, latestSourceComponentVersion.Original())
	if err != nil {
		err := fmt.Errorf("failed to get latest component version: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.GetComponentDescriptorFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	defer func() {
		if err := sourceComponentVersion.Close(); err != nil {
			logger.Error(err, "failed to close source component version, context might be leaking...")
		}
	}()

	logger.V(4).Info("pulling", "component-name", sourceComponentVersion.GetName())

	if obj.Spec.Destination != nil {
		if err := r.OCMClient.TransferComponent(ctx, octx, obj, sourceComponentVersion, latestSourceComponentVersion.Original()); err != nil {
			err := fmt.Errorf("failed to transfer components: %w", err)
			conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.TransferFailedReason, err.Error())

			logger.Error(err, "transferring components failed")
			return ctrl.Result{}, err
		}

		obj.Status.ReplicatedRepositoryURL = obj.Spec.Destination.URL
	} else {
		logger.Info("skipping transferring as no destination is provided for source component", "component-name", sourceComponentVersion.GetName())

		obj.Status.ReplicatedRepositoryURL = obj.Spec.Source.URL
	}

	// Update the replicated version to the latest version
	obj.Status.LastAppliedVersion = latestSourceComponentVersion.Original()

	logger.Info("resource is ready")
	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	// Always requeue to constantly check for new versions.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComponentSubscription{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func (r *ComponentSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		result ctrl.Result
		retErr error
	)

	logger := log.FromContext(ctx)
	obj := &v1alpha1.ComponentSubscription{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("component deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	logger = logger.WithValues("subscription", klog.KObj(obj))
	logger.Info("starting reconcile loop")

	if obj.DeletionTimestamp != nil {
		logger.Info("subscription is being deleted...")
		return ctrl.Result{}, nil
	}

	// The replication controller doesn't need a shouldReconcile, because it should always reconcile,
	// that is its purpose.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		retErr = errors.Join(retErr, err)
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.PatchFailedReason, err.Error())
		return ctrl.Result{}, retErr
	}

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if condition := conditions.Get(obj, meta.StalledCondition); condition != nil && condition.Status == metav1.ConditionTrue {
			conditions.Delete(obj, meta.ReconcilingCondition)
		}

		// Check if it's a successful reconciliation.
		// We don't set Requeue in case of error, so we can safely check for Requeue.
		if result.RequeueAfter == obj.GetRequeueAfter() && !result.Requeue && retErr == nil {
			// Remove the reconciling condition if it's set.
			conditions.Delete(obj, meta.ReconcilingCondition)

			// Set the return err as the ready failure message if the resource is not ready, but also not reconciling or stalled.
			if ready := conditions.Get(obj, meta.ReadyCondition); ready != nil && ready.Status == metav1.ConditionFalse && !conditions.IsStalled(obj) {
				retErr = errors.New(conditions.GetMessage(obj, meta.ReadyCondition))
			}
		}

		// If still reconciling then reconciliation did not succeed, set to ProgressingWithRetry to
		// indicate that reconciliation will be retried.
		if conditions.IsReconciling(obj) {
			reconciling := conditions.Get(obj, meta.ReconcilingCondition)
			reconciling.Reason = meta.ProgressingWithRetryReason
			conditions.Set(obj, reconciling)
		}

		// If not reconciling or stalled than mark Ready=True
		if !conditions.IsReconciling(obj) &&
			!conditions.IsStalled(obj) &&
			retErr == nil &&
			result.RequeueAfter == obj.GetRequeueAfter() {
			conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
		}
		// Set status observed generation option if the component is stalled or ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			obj.Status.ObservedGeneration = obj.Generation
		}

		// Update the object.
		if err := patchHelper.Patch(ctx, obj); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()

	result, retErr = r.reconcile(ctx, obj)
	return result, retErr
}

func (r *ComponentSubscriptionReconciler) reconcile(ctx context.Context, obj *v1alpha1.ComponentSubscription) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to authenticate OCM context: %w", err)
	}

	version, err := r.OCMClient.GetLatestSourceComponentVersion(ctx, octx, obj)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.PullingLatestVersionFailedReason, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to get latest component version: %w", err)
	}
	logger.V(4).Info("got newest version from component", "version", version)

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if version == obj.Status.LastAppliedVersion {
		logger.Info("latest version and replicated version are a match and not empty")
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	latestSourceComponentVersion, err := semver.NewVersion(version)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse version: %w", err)
	}

	subReplicated := "0.0.0"
	if obj.Status.LastAppliedVersion != "" {
		subReplicated = obj.Status.LastAppliedVersion
	}

	replicatedVersion, err := semver.NewVersion(subReplicated)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.SemverConversionFailedReason, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to parse latest version: %w", err)
	}

	logger.V(4).Info("latest replicated version is", "replicated", replicatedVersion.Original())

	if latestSourceComponentVersion.LessThan(replicatedVersion) || latestSourceComponentVersion.Equal(replicatedVersion) {
		logger.Info("no new version found", "version", latestSourceComponentVersion.Original(), "latest", replicatedVersion.Original())
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// set latest version, this will be patched in the defer statement.
	obj.Status.LastAttemptedVersion = latestSourceComponentVersion.Original()
	obj.Status.LastAppliedVersion = replicatedVersion.Original()

	sourceComponentVersion, err := r.OCMClient.GetComponentVersion(ctx, octx, obj, latestSourceComponentVersion.Original())
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.GetComponentDescriptorFailedReason, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to get latest component version: %w", err)
	}

	defer func() {
		if err := sourceComponentVersion.Close(); err != nil {
			logger.Error(err, "failed to close source component version, context might be leaking...")
		}
	}()

	logger.V(4).Info("pulling", "component-name", sourceComponentVersion.GetName())

	if obj.Spec.Destination != nil {
		if err := r.OCMClient.TransferComponent(ctx, octx, obj, sourceComponentVersion, latestSourceComponentVersion.Original()); err != nil {
			conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.TransferFailedReason, err.Error())
			logger.Error(err, "transferring components failed")
			return ctrl.Result{}, fmt.Errorf("failed to transfer components: %w", err)
		}

		obj.Status.ReplicatedRepositoryURL = obj.Spec.Destination.URL
	} else {
		logger.Info("skipping transferring as no destination is provided for source component", "component-name", sourceComponentVersion.GetName())

		obj.Status.ReplicatedRepositoryURL = obj.Spec.Source.URL
	}

	// Update the replicated version to the latest version
	obj.Status.LastAppliedVersion = latestSourceComponentVersion.Original()

	// Remove any stale Ready condition, most likely False, set above. Its value
	// is derived from the overall result of the reconciliation in the deferred
	// block at the very end.
	conditions.Delete(obj, meta.ReadyCondition)

	// Always requeue to constantly check for new versions.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComponentSubscription{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithEventFilter(predicate.Or(SubscriptionUpdatedPredicate{})).
		Complete(r)
}

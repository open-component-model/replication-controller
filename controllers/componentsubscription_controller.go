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
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/open-component-model/ocm-controller/pkg/event"
	"github.com/open-component-model/ocm-controller/pkg/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/open-component-model/replication-controller/pkg/ocm"
)

// ComponentSubscriptionReconciler reconciles a ComponentSubscription object
type ComponentSubscriptionReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OCMClient     ocm.Contract
	EventRecorder record.EventRecorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	const (
		sourceKey      = ".metadata.source.secretRef"
		destinationKey = ".metadata.destination.secretRef"
	)

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v1alpha1.ComponentSubscription{}, sourceKey, func(rawObj client.Object) []string {
		obj, ok := rawObj.(*v1alpha1.ComponentSubscription)
		if !ok {
			return []string{}
		}
		if obj.Spec.Source.SecretRef == nil {
			return []string{}
		}

		ns := obj.GetNamespace()
		return []string{fmt.Sprintf("%s/%s", ns, obj.Spec.Source.SecretRef.Name)}
	}); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v1alpha1.ComponentSubscription{}, destinationKey, func(rawObj client.Object) []string {
		obj, ok := rawObj.(*v1alpha1.ComponentSubscription)
		if !ok {
			return []string{}
		}
		if obj.Spec.Destination.SecretRef == nil {
			return []string{}
		}

		ns := obj.GetNamespace()
		return []string{fmt.Sprintf("%s/%s", ns, obj.Spec.Destination.SecretRef.Name)}
	}); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComponentSubscription{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.findObjects(sourceKey, destinationKey))).
		Complete(r)
}

// findObjects finds component versions that have a key for the secret that triggered this watch event.
func (r *ComponentSubscriptionReconciler) findObjects(sourceKey string, destinationKey string) handler.MapFunc {
	return func(obj client.Object) []reconcile.Request {
		sourceList := &v1alpha1.ComponentSubscriptionList{}
		if err := r.List(context.Background(), sourceList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(sourceKey, client.ObjectKeyFromObject(obj).String()),
		}); err != nil {
			return []reconcile.Request{}
		}

		destinationList := &v1alpha1.ComponentSubscriptionList{}
		if err := r.List(context.Background(), destinationList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(destinationKey, client.ObjectKeyFromObject(obj).String()),
		}); err != nil {
			return []reconcile.Request{}
		}

		// deduplicate the two secret lists
		requestMap := make(map[reconcile.Request]struct{})
		for _, item := range sourceList.Items {
			requestMap[reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			}] = struct{}{}
		}

		for _, item := range destinationList.Items {
			requestMap[reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			}] = struct{}{}
		}

		requests := make([]reconcile.Request, len(requestMap))
		for k := range requestMap {
			requests = append(requests, k)
		}

		return requests
	}
}

//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ComponentSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	obj := &v1alpha1.ComponentSubscription{}
	if err = r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if obj.DeletionTimestamp != nil {
		return
	}

	// The replication controller doesn't need a shouldReconcile, because it should always reconcile,
	// that is its purpose.
	patchHelper := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if derr := status.UpdateStatus(ctx, patchHelper, obj, r.EventRecorder, obj.GetRequeueAfter()); derr != nil {
			err = errors.Join(err, derr)
		}
	}()

	// Starts the progression by setting ReconcilingCondition.
	// This will be checked in defer.
	// Should only be deleted on a success.
	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "reconciliation in progress for resource: %s", obj.Name)

	return r.reconcile(ctx, obj)
}

func (r *ComponentSubscriptionReconciler) reconcile(ctx context.Context, obj *v1alpha1.ComponentSubscription) (_ ctrl.Result, err error) {
	if obj.Generation != obj.Status.ObservedGeneration {
		rreconcile.ProgressiveStatus(
			false,
			obj,
			meta.ProgressingReason,
			"processing object: new generation %d -> %d",
			obj.Status.ObservedGeneration,
			obj.Generation,
		)
	}

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj)
	if err != nil {
		err := fmt.Errorf("failed to authenticate OCM context: %w", err)
		status.MarkAsStalled(r.EventRecorder, obj, v1alpha1.AuthenticationFailedReason, err.Error())

		return ctrl.Result{}, nil
	}

	version, err := r.OCMClient.GetLatestSourceComponentVersion(ctx, octx, obj)
	if err != nil {
		err := fmt.Errorf("failed to get latest component version: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.PullingLatestVersionFailedReason, err.Error())

		// we don't want to fail but keep searching until it's there. But we do mark the subscription as failed.
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if version == obj.Status.LastAppliedVersion {
		r.markAsDone(obj)

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	latestSourceComponentVersion, err := semver.NewVersion(version)
	if err != nil {
		err := fmt.Errorf("failed to parse source component version: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.SemverConversionFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	lastAppliedOriginal := "0.0.0"
	if obj.Status.LastAppliedVersion != "" {
		lastAppliedOriginal = obj.Status.LastAppliedVersion
	}

	lastAppliedVersion, err := semver.NewVersion(lastAppliedOriginal)
	if err != nil {
		err := fmt.Errorf("failed to parse latest version: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.SemverConversionFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	if latestSourceComponentVersion.LessThan(lastAppliedVersion) || latestSourceComponentVersion.Equal(lastAppliedVersion) {
		r.markAsDone(obj)

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// set latest version, this will be patched in the defer statement.
	obj.Status.LastAttemptedVersion = latestSourceComponentVersion.Original()

	sourceComponentVersion, err := r.OCMClient.GetComponentVersion(ctx, octx, obj, latestSourceComponentVersion.Original())
	if err != nil {
		err := fmt.Errorf("failed to get latest component version: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.GetComponentDescriptorFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	defer func() {
		if cerr := sourceComponentVersion.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	if obj.Spec.Destination != nil {
		rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "transferring component to target repository: %s", obj.Spec.Destination.URL)

		if err := r.OCMClient.TransferComponent(ctx, octx, obj, sourceComponentVersion, latestSourceComponentVersion.Original()); err != nil {
			err := fmt.Errorf("failed to transfer components: %w", err)
			status.MarkNotReady(r.EventRecorder, obj, v1alpha1.TransferFailedReason, err.Error())

			return ctrl.Result{}, err
		}

		obj.Status.ReplicatedRepositoryURL = obj.Spec.Destination.URL
	} else {
		obj.Status.ReplicatedRepositoryURL = obj.Spec.Source.URL
	}

	// Update the replicated version to the latest version
	obj.Status.LastAppliedVersion = latestSourceComponentVersion.Original()

	r.markAsDone(obj)

	// Always requeue to constantly check for new versions.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

func (r *ComponentSubscriptionReconciler) markAsDone(obj *v1alpha1.ComponentSubscription) {
	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
	event.New(r.EventRecorder, obj, eventv1.EventSeverityInfo, "Reconciliation success", nil)
}

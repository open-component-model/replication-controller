// Copyright 2022.
// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	csdk "github.com/open-component-model/ocm-controllers-sdk"
	ocmreg "github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer/transferhandler/standard"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/ocm/pkg/contexts/oci/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/attrs/signingattr"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/signing"
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
	var (
		result ctrl.Result
		retErr error
	)

	log := log.FromContext(ctx)
	obj := &v1alpha1.ComponentSubscription{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("component deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	log = log.WithValues("subscription", klog.KObj(obj))
	log.Info("starting reconcile loop")

	if obj.DeletionTimestamp != nil {
		log.Info("subscription is being deleted...")
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
	return ctrl.Result{}, retErr
}

func (r *ComponentSubscriptionReconciler) reconcile(ctx context.Context, obj *v1alpha1.ComponentSubscription) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	session := ocm.NewSession(nil)
	defer session.Close()

	ocmCtx := ocm.ForContext(ctx)
	if obj.Spec.Source.SecretRef != nil {
		if err := csdk.ConfigureCredentials(ctx, ocmCtx, r.Client, obj.Spec.Source.URL, obj.Spec.Source.SecretRef.Name, obj.Namespace); err != nil {
			log.Error(err, "failed to find credentials")
			return ctrl.Result{}, fmt.Errorf("failed to configure credentials for component: %w", err)
		}
	}

	version, err := r.pullLatestVersion(ocmCtx, session, *obj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get latest component version: %w", err)
	}
	log.V(4).Info("got newest version from component", "version", version)

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if version == obj.Status.ReplicatedVersion {
		log.Info("latest version and replicated version are a match and not empty")
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	constraint, err := semver.NewConstraint(obj.Spec.Semver)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse semver constraint: %w", err)
	}
	current, err := semver.NewVersion(version)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse version: %w", err)
	}

	subReplicated := "0.0.0"
	if obj.Status.ReplicatedVersion != "" {
		subReplicated = obj.Status.ReplicatedVersion
	}

	replicatedVersion, err := semver.NewVersion(subReplicated)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse latest version: %w", err)
	}

	log.V(4).Info("latest replicated version is", "replicated", replicatedVersion.Original())

	if !constraint.Check(current) {
		log.Info("version did not satisfy constraint, skipping...", "version", version, "constraint", constraint.String())
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if current.LessThan(replicatedVersion) || current.Equal(replicatedVersion) {
		log.Info("no new version found", "version", current.Original(), "latest", replicatedVersion.Original())
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// set latest version, this will be patched in the defer statement.
	obj.Status.LatestVersion = current.Original()
	obj.Status.ReplicatedVersion = replicatedVersion.Original()

	sourceComponentVersion, err := csdk.GetComponentVersion(ocmCtx, session, obj.Spec.Source.URL, obj.Spec.Component, current.Original())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get latest component descriptor: %w", err)
	}
	log.V(4).Info("pulling", "component-name", sourceComponentVersion.GetName())

	if obj.Spec.Destination.SecretRef != nil {
		if err := csdk.ConfigureCredentials(ctx, ocmCtx, r.Client, obj.Spec.Destination.URL, obj.Spec.Destination.SecretRef.Name, obj.Namespace); err != nil {
			log.Error(err, "failed to find credentials for destination")
			return ctrl.Result{}, fmt.Errorf("failed to configure credentials for component: %w", err)
		}
	}

	source, err := ocmCtx.RepositoryForSpec(ocmreg.NewRepositorySpec(obj.Spec.Source.URL, nil))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get source repo: %w", err)
	}

	ok, err := r.verifyComponent(ctx, ocmCtx, source, sourceComponentVersion, obj.Namespace, obj.Spec.Verify)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to verify signature: %w", err)
	}
	if !ok {
		return ctrl.Result{}, fmt.Errorf("on of the signatures failed to match: %w", err)
	}

	target, err := ocmCtx.RepositoryForSpec(ocmreg.NewRepositorySpec(obj.Spec.Destination.URL, nil))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get target repo: %w", err)
	}

	handler, err := standard.New(
		standard.Recursive(true),
		standard.ResourcesByValue(true),
		standard.Overwrite(true),
		standard.Resolver(source),
		standard.Resolver(target),
		// if additional resolvers are required they could be added here...
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct target handler: %w", err)
	}
	if err := transfer.TransferVersion(
		nil,
		transfer.TransportClosure{},
		sourceComponentVersion,
		target,
		handler,
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to transfer version to destination repository: %w", err)
	}

	// Update the replicated version to the latest version
	obj.Status.ReplicatedVersion = current.Original()

	// Always requeue to constantly check for new versions.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ComponentSubscription{}).
		WithEventFilter(predicate.Or(SubscriptionUpdatedPredicate{})).
		Complete(r)
}

func (r *ComponentSubscriptionReconciler) pullLatestVersion(ocmCtx ocm.Context, session ocm.Session, subscription v1alpha1.ComponentSubscription) (string, error) {
	versions, err := r.listComponentVersions(ocmCtx, session, subscription)
	if err != nil {
		return "", fmt.Errorf("failed to get component versions: %w", err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for component '%s'", subscription.Spec.Component)
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].GreaterThan(versions[j])
	})

	return versions[0].Original(), nil
}

func (r *ComponentSubscriptionReconciler) listComponentVersions(ctx ocm.Context, session ocm.Session, subscription v1alpha1.ComponentSubscription) ([]*semver.Version, error) {
	// configure the repository access
	repoSpec := genericocireg.NewRepositorySpec(ocireg.NewRepositorySpec(subscription.Spec.Source.URL), nil)
	repo, err := session.LookupRepository(ctx, repoSpec)
	if err != nil {
		return nil, fmt.Errorf("repo error: %w", err)
	}

	// get the component version
	cv, err := session.LookupComponent(repo, subscription.Spec.Component)
	if err != nil {
		return nil, fmt.Errorf("component error: %w", err)
	}

	versions, err := cv.ListVersions()
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for component: %w", err)
	}
	var result []*semver.Version
	for _, v := range versions {
		parsed, err := semver.NewVersion(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse version '%s': %w", v, err)
		}
		result = append(result, parsed)
	}
	return result, nil
}

func (r *ComponentSubscriptionReconciler) verifyComponent(ctx context.Context, ocmCtx ocm.Context, repo ocm.Repository, cv ocm.ComponentVersionAccess, namespace string, signatures []v1alpha1.Signature) (bool, error) {
	log := log.FromContext(ctx)
	resolver := ocm.NewCompoundResolver(repo)

	for _, signature := range signatures {
		cert, err := r.getPublicKey(ctx, namespace, signature.PublicKey.SecretRef.Name, signature.Name)
		if err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		opts := signing.NewOptions(
			signing.VerifySignature(signature.Name),
			signing.Resolver(resolver),
			signing.VerifyDigests(),
			signing.PublicKey(signature.Name, cert),
		)

		if err := opts.Complete(signingattr.Get(ocmCtx)); err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		dig, err := signing.Apply(nil, nil, cv, opts)
		if err != nil {
			return false, err
		}

		var value string
		for _, os := range cv.GetDescriptor().Signatures {
			if os.Name == signature.Name {
				value = os.Digest.Value
				break
			}
		}
		if value == "" {
			return false, fmt.Errorf("signature with name '%s' not found in the list of provided ocm signatures", signature.Name)
		}
		log.V(4).Info("comparing", "dig-value", dig.Value, "descriptor-value", value)
		if dig.Value != value {
			return false, fmt.Errorf("%s signature did not match key value", signature.Name)
		}
	}
	return true, nil
}

func (r *ComponentSubscriptionReconciler) getPublicKey(ctx context.Context, namespace, name, signature string) ([]byte, error) {
	var secret corev1.Secret
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		return nil, err
	}

	for key, value := range secret.Data {
		if key == signature {
			return value, nil
		}
	}

	return nil, errors.New("public key not found")
}

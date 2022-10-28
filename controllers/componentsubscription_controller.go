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
	"fmt"
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	csdk "github.com/open-component-model/ocm-controllers-sdk"
	"github.com/open-component-model/ocm/cmds/ocm/commands/common/options/closureoption"
	"github.com/open-component-model/ocm/cmds/ocm/commands/ocmcmds/common/options/lookupoption"
	"github.com/open-component-model/ocm/cmds/ocm/commands/ocmcmds/common/options/overwriteoption"
	"github.com/open-component-model/ocm/cmds/ocm/commands/ocmcmds/common/options/rscbyvalueoption"
	"github.com/open-component-model/ocm/pkg/contexts/oci/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
	ocmreg "github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer/transferhandler/spiff"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
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
			log.Info("component deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	log = log.WithValues("subscription", klog.KObj(subscription))
	log.Info("starting reconcile loop")

	if subscription.DeletionTimestamp != nil {
		log.Info("subscription is being deleted...")
		return ctrl.Result{}, nil
	}

	interval, err := time.ParseDuration(subscription.Spec.Interval)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse interval: %w", err)
	}
	requeue := func() ctrl.Result {
		return ctrl.Result{
			RequeueAfter: interval,
		}
	}

	session := ocm.NewSession(nil)
	defer session.Close()

	ocmCtx := ocm.ForContext(ctx)
	// configure credentials
	if err := csdk.ConfigureCredentials(ctx, ocmCtx, r.Client, subscription.Spec.Source.URL, subscription.Spec.Source.SecretRef.Name, subscription.Namespace); err != nil {
		log.Error(err, "failed to find credentials")
		return requeue(), fmt.Errorf("failed to configure credentials for component: %w", err)
	}

	version, err := r.pullLatestVersion(ocmCtx, session, *subscription)
	if err != nil {
		return requeue(), fmt.Errorf("failed to get latest component version: %w", err)
	}
	log.V(4).Info("got newest version from component", "version", version)

	// Because of the predicate, this subscription will be reconciled again once there is an update to its status field.
	if version == subscription.Status.ReplicatedVersion {
		log.Info("latest version and replicated version are a match and not empty")
		return requeue(), nil
	}

	constraint, err := semver.NewConstraint(subscription.Spec.Semver)
	if err != nil {
		return requeue(), fmt.Errorf("failed to parse semver constraint: %w", err)
	}
	current, err := semver.NewVersion(version)
	if err != nil {
		return requeue(), fmt.Errorf("failed to parse version: %w", err)
	}
	subReplicated := "0.0.0"
	if subscription.Status.ReplicatedVersion != "" {
		subReplicated = subscription.Status.ReplicatedVersion
	}
	replicatedVersion, err := semver.NewVersion(subReplicated)
	if err != nil {
		return requeue(), fmt.Errorf("failed to parse latest version: %w", err)
	}
	log.V(4).Info("latest replicated version is", "replicated", replicatedVersion.String())
	if !constraint.Check(current) {
		log.Info("version did not satisfy constraint, skipping...", "version", version, "constraint", constraint.String())
		return requeue(), nil
	}
	if current.LessThan(replicatedVersion) || current.Equal(replicatedVersion) {
		log.Info("no new version found", "version", current.String(), "latest", replicatedVersion.String())
		return requeue(), nil
	}

	sourceComponentVersion, err := csdk.GetComponentVersion(ocmCtx, session, subscription.Spec.Source.URL, subscription.Spec.Component, current.String())
	if err != nil {
		return requeue(), fmt.Errorf("failed to get latest component descriptor: %w", err)
	}
	log.V(4).Info("pulling", "component-name", sourceComponentVersion.GetName())

	targetOcmCtx := ocm.ForContext(ctx)
	// configure credentials
	if err := csdk.ConfigureCredentials(ctx, targetOcmCtx, r.Client, subscription.Spec.Destination.URL, subscription.Spec.Destination.SecretRef.Name, subscription.Namespace); err != nil {
		log.Error(err, "failed to find credentials for destination")
		return requeue(), fmt.Errorf("failed to configure credentials for component: %w", err)
	}

	repoSpec := ocmreg.NewRepositorySpec(subscription.Spec.Destination.URL, nil)
	target, err := targetOcmCtx.RepositoryForSpec(repoSpec)
	if err != nil {
		return requeue(), fmt.Errorf("failed to get target repo: %w", err)
	}

	thdlr, err := spiff.New(
		rscbyvalueoption.New(),
		closureoption.New("component reference"),
		overwriteoption.New(),
		rscbyvalueoption.New(),
		lookupoption.New(),
	)
	if err != nil {
		return requeue(), fmt.Errorf("failed to construct target handler: %w", err)
	}
	if err := transfer.TransferVersion(
		nil,
		transfer.TransportClosure{},
		sourceComponentVersion,
		target,
		thdlr,
	); err != nil {
		return requeue(), fmt.Errorf("failed to transfer version to destination repositroy: %w", err)
	}

	newSub := subscription.DeepCopy()
	newSub.Status.LatestVersion = current.String()
	newSub.Status.ReplicatedVersion = current.String()
	if err := patchObject(ctx, r.Client, subscription, newSub); err != nil {
		return requeue(), fmt.Errorf("failed to patch subscription: %w", err)
	}

	// Always requeue to constantly check for new versions.
	return requeue(), nil
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

	return versions[0].String(), nil
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
	fmt.Println("COMPONENT: ", cv.GetName())
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

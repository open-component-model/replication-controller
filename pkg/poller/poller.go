package poller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	csdk "github.com/open-component-model/ocm-controllers-sdk"
	"github.com/open-component-model/ocm/pkg/contexts/oci/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
	"github.com/open-component-model/replication-controller/pkg/patcher"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/open-component-model/replication-controller/pkg/providers/oci"
)

var (
	// AnnotationPollerRunning defines if a poller has already been started for a subscription.
	AnnotationPollerRunning = "poller-running"

	annotationPollerFailed    = "poller-failed"
	annotationPollerStop      = "poller-stop"
	maximumFailedPullAttempts = 5
)

type Poller struct {
	Client       client.Client
	OciClient    oci.Client
	Subscription *v1alpha1.ComponentSubscription
	Logger       logr.Logger
}

// NewPoller creates a new version poller.
func NewPoller(log logr.Logger, client client.Client, sub *v1alpha1.ComponentSubscription) *Poller {
	return &Poller{
		Client:       client,
		Subscription: sub,
		Logger:       log.WithName("poller"),
	}
}

func (p *Poller) Poll(ctx context.Context) error {
	interval, err := time.ParseDuration(p.Subscription.Spec.Interval)
	if err != nil {
		return fmt.Errorf("failed to parse time: %w", err)
	}
	constraint, err := semver.NewConstraint(p.Subscription.Spec.Semver)
	if err != nil {
		return fmt.Errorf("failed to parse semver constraint: %w", err)
	}
	newSub := p.Subscription.DeepCopy()
	newSub.Annotations[AnnotationPollerRunning] = "true"
	if err := patcher.PatchObject(ctx, p.Client, p.Subscription, newSub); err != nil {
		return fmt.Errorf("failed to add poller annotation to subscription: %w", err)
	}
	// Once it's successful, update the internal subscription, so it contains the annotation.
	p.Subscription = newSub

	go p.run(ctx, interval, constraint)
	return nil
}

func (p *Poller) run(ctx context.Context, interval time.Duration, constraint *semver.Constraints) {
	log := p.Logger.WithValues("subscription", klog.KObj(p.Subscription))
	log.Info("started poller for subscription")
	ticker := time.After(interval)

	failedPullAttempts := 0
	for {
		select {
		case <-ticker:
			log.V(4).Info("tick for subscription")
			sub := &v1alpha1.ComponentSubscription{}
			if err := p.Client.Get(ctx, types.NamespacedName{
				Namespace: p.Subscription.Namespace,
				Name:      p.Subscription.Name,
			}, sub); err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("stopping poller for subscription as it was deleted")
					return
				}
				log.Error(err, "encountered an error while trying to poll for new versions for subscription")
				return
			}
			// Check to see if the poller has to be stopped for this subscription
			if _, ok := sub.Annotations[annotationPollerStop]; ok {
				log.Info("encountered annotation to stop polling for subscription... stopping.")
				return
			}
			version, err := p.pullVersion(ctx)
			if err != nil {
				log.Error(err, "failed to pull new version for component, retrying in a bit...", "component", sub.Spec.Component)
				failedPullAttempts++
				if failedPullAttempts == maximumFailedPullAttempts {
					log.Error(err, "maximum number of failed pull attempts reached, cancelling the poller")
					return
				}
				break
			} else {
				failedPullAttempts = 0
			}

			current, err := semver.NewVersion(version)
			if err != nil {
				log.Error(err, "failed to parse version, checking back later")
				break
			}
			// TODO: think about how to handle this nicely.
			if sub.Status.LatestVersion == "" {
				sub.Status.LatestVersion = "0.0.0"
			}
			latest, err := semver.NewVersion(sub.Status.LatestVersion)
			if err != nil {
				log.Error(err, "failed to parse latest version, checking back later")
				break
			}
			if !constraint.Check(current) {
				log.Info("version did not satisfy constraint, skipping...", "version", version, "constraint", constraint.String())
				break
			}
			if current.LessThan(latest) || current.Equal(latest) {
				log.Info("no new versions found", "version", current.String(), "latest", latest.String())
				break
			}
		case <-ctx.Done():
			log.Error(ctx.Err(), "context was cancelled for this poller")
			return
		}
	}
}

func (p *Poller) pullVersion(ctx context.Context) (string, error) {
	session := ocm.NewSession(nil)
	defer session.Close()

	ocmCtx := ocm.ForContext(ctx)
	// configure credentials
	if err := csdk.ConfigureCredentials(ctx, ocmCtx, p.Client, p.Subscription.Spec.Source.URL, p.Subscription.Spec.Source.SecretRef.Name, p.Subscription.Namespace); err != nil {
		p.Logger.Error(err, "failed to find credentials")
		// ignore not found errors for now
		if !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to configure credentials for component: %w", err)
		}
	}

	versions, err := p.listComponentVersions(ocmCtx, session)
	if err != nil {
		return "", fmt.Errorf("failed to get component versions: %w", err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for component '%s'", p.Subscription.Spec.Component)
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].GreaterThan(versions[j])
	})

	return versions[0].String(), nil
}

func (p *Poller) listComponentVersions(ctx ocm.Context, session ocm.Session) ([]*semver.Version, error) {
	// configure the repository access
	repoSpec := genericocireg.NewRepositorySpec(ocireg.NewRepositorySpec(p.Subscription.Spec.Source.URL), nil)
	repo, err := session.LookupRepository(ctx, repoSpec)
	if err != nil {
		return nil, fmt.Errorf("repo error: %w", err)
	}

	// get the component version
	cv, err := session.LookupComponent(repo, p.Subscription.Spec.Component)
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

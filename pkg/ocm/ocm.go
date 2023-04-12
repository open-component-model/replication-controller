// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package ocm

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/go-logr/logr"
	"github.com/open-component-model/ocm/pkg/contexts/credentials/repositories/dockerconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/attrs/signingattr"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/signing"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/transfer/transferhandler/standard"

	csdk "github.com/open-component-model/ocm-controllers-sdk"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
)

// Verifier takes a Component and runs OCM verification on it.
type Verifier interface {
	VerifySourceComponent(ctx context.Context, obj *v1alpha1.ComponentSubscription, version string) (bool, error)
}

// Fetcher gets information about an OCM component Version based on a k8s component Version.
type Fetcher interface {
	GetComponentVersion(ctx context.Context, obj *v1alpha1.ComponentSubscription, version string) (ocm.ComponentVersionAccess, error)
	GetLatestSourceComponentVersion(ctx context.Context, obj *v1alpha1.ComponentSubscription) (string, error)
	TransferComponent(ctx context.Context, obj *v1alpha1.ComponentSubscription, sourceComponentVersion ocm.ComponentVersionAccess, version string) error
}

// FetchVerifier can fetch and verify components.
type FetchVerifier interface {
	Verifier
	Fetcher
}

// Client implements the OCM fetcher interface.
type Client struct {
	client client.Client
}

var _ FetchVerifier = &Client{}

// NewClient creates a new fetcher Client using the provided k8s client.
func NewClient(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

// GetComponentVersion returns a component Version. It's the caller's responsibility to clean it up and close the component Version once done with it.
func (c *Client) GetComponentVersion(ctx context.Context, obj *v1alpha1.ComponentSubscription, version string) (ocm.ComponentVersionAccess, error) {
	octx := ocm.ForContext(ctx)

	if err := c.maybeConfigureServiceAccountAccess(ctx, octx, obj); err != nil {
		return nil, fmt.Errorf("failed to configure service account access: %w", err)
	}

	if err := c.maybeConfigureAccessCredentials(ctx, octx, obj.Spec.Source, obj.Namespace); err != nil {
		return nil, fmt.Errorf("failed to configure credentials for destination: %w", err)
	}

	repoSpec := ocireg.NewRepositorySpec(obj.Spec.Source.URL, nil)
	repo, err := octx.RepositoryForSpec(repoSpec)
	if err != nil {
		panic(err)
	}
	defer repo.Close()

	cv, err := repo.LookupComponentVersion(obj.Spec.Component, version)
	if err != nil {
		return nil, fmt.Errorf("failed to look up component Version: %w", err)
	}

	return cv, nil
}

func (c *Client) VerifySourceComponent(ctx context.Context, obj *v1alpha1.ComponentSubscription, version string) (bool, error) {
	log := log.FromContext(ctx)

	octx := ocm.ForContext(ctx)

	if err := c.maybeConfigureServiceAccountAccess(ctx, octx, obj); err != nil {
		return false, fmt.Errorf("failed to configure service account access: %w", err)
	}

	if err := c.maybeConfigureAccessCredentials(ctx, octx, obj.Spec.Source, obj.Namespace); err != nil {
		return false, fmt.Errorf("failed to configure credentials for destination: %w", err)
	}

	repoSpec := ocireg.NewRepositorySpec(obj.Spec.Source.URL, nil)
	repo, err := octx.RepositoryForSpec(repoSpec)
	if err != nil {
		panic(err)
	}
	defer repo.Close()

	cv, err := repo.LookupComponentVersion(obj.Spec.Component, version)
	if err != nil {
		return false, fmt.Errorf("failed to look up component Version: %w", err)
	}
	defer cv.Close()

	resolver := ocm.NewCompoundResolver(repo)

	for _, signature := range obj.Spec.Verify {
		cert, err := c.getPublicKey(ctx, obj.Namespace, signature.PublicKey.SecretRef.Name, signature.Name)
		if err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		opts := signing.NewOptions(
			signing.Resolver(resolver),
			signing.PublicKey(signature.Name, cert),
			signing.VerifyDigests(),
			signing.VerifySignature(signature.Name),
		)

		if err := opts.Complete(signingattr.Get(octx)); err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		dig, err := signing.Apply(nil, nil, cv, opts)
		if err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		var value string
		for _, s := range cv.GetDescriptor().Signatures {
			if s.Name == signature.Name {
				value = s.Digest.Value
				break
			}
		}

		if value == "" {
			return false, fmt.Errorf("signature with name '%s' not found in the list of provided ocm signatures", signature.Name)
		}

		if dig.Value != value {
			return false, fmt.Errorf("%s signature did not match key value", signature.Name)
		}

		log.Info("component verified", "signature", signature.Name)
	}

	return true, nil
}

func (c *Client) getPublicKey(ctx context.Context, namespace, name, signature string) ([]byte, error) {
	var secret corev1.Secret
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.client.Get(ctx, secretKey, &secret); err != nil {
		return nil, err
	}

	for key, value := range secret.Data {
		if key == signature {
			return value, nil
		}
	}

	return nil, errors.New("public key not found")
}

func (c *Client) GetLatestSourceComponentVersion(ctx context.Context, obj *v1alpha1.ComponentSubscription) (string, error) {
	log := log.FromContext(ctx)

	octx := ocm.ForContext(ctx)

	if err := c.maybeConfigureServiceAccountAccess(ctx, octx, obj); err != nil {
		return "", fmt.Errorf("failed to configure service account access: %w", err)
	}

	if err := c.maybeConfigureAccessCredentials(ctx, octx, obj.Spec.Source, obj.Namespace); err != nil {
		return "", fmt.Errorf("failed to configure credentials for destination: %w", err)
	}

	versions, err := c.listComponentVersions(log, octx, obj)
	if err != nil {
		return "", fmt.Errorf("failed to get component versions: %w", err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for component '%s'", obj.Spec.Component)
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].Semver.GreaterThan(versions[j].Semver)
	})

	constraint, err := semver.NewConstraint(obj.Spec.Semver)
	if err != nil {
		return "", fmt.Errorf("failed to parse constraint version: %w", err)
	}

	for _, v := range versions {
		if valid, _ := constraint.Validate(v.Semver); valid {
			return v.Version, nil
		}
	}

	return "", fmt.Errorf("no matching versions found for constraint '%s'", obj.Spec.Semver)
}

// Version has two values to be able to sort a list but still return the actual Version.
// The Version might contain a `v`.
type Version struct {
	Semver  *semver.Version
	Version string
}

func (c *Client) listComponentVersions(logger logr.Logger, octx ocm.Context, obj *v1alpha1.ComponentSubscription) ([]Version, error) {
	repoSpec := ocireg.NewRepositorySpec(obj.Spec.Source.URL, nil)
	repo, err := octx.RepositoryForSpec(repoSpec)
	if err != nil {
		panic(err)
	}
	defer repo.Close()

	// get the component Version
	cv, err := repo.LookupComponent(obj.Spec.Component)
	if err != nil {
		return nil, fmt.Errorf("component error: %w", err)
	}
	defer cv.Close()

	versions, err := cv.ListVersions()
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for component: %w", err)
	}

	var result []Version
	for _, v := range versions {
		parsed, err := semver.NewVersion(v)
		if err != nil {
			logger.Error(err, "skipping invalid version", "version", v)
			continue
		}
		result = append(result, Version{
			Semver:  parsed,
			Version: v,
		})
	}
	return result, nil
}

func (c *Client) TransferComponent(ctx context.Context, obj *v1alpha1.ComponentSubscription, sourceComponentVersion ocm.ComponentVersionAccess, version string) error {
	octx := ocm.ForContext(ctx)

	if err := c.maybeConfigureAccessCredentials(ctx, octx, obj.Spec.Source, obj.Namespace); err != nil {
		return fmt.Errorf("failed to configure credentials for destination: %w", err)
	}

	if err := c.maybeConfigureAccessCredentials(ctx, octx, *obj.Spec.Destination, obj.Namespace); err != nil {
		return fmt.Errorf("failed to configure credentials for destination: %w", err)
	}

	sourceRepoSpec := ocireg.NewRepositorySpec(obj.Spec.Source.URL, nil)
	source, err := octx.RepositoryForSpec(sourceRepoSpec)
	if err != nil {
		panic(err)
	}
	defer source.Close()

	ok, err := c.VerifySourceComponent(ctx, obj, version)
	if err != nil {
		return fmt.Errorf("failed to verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("on of the signatures failed to match: %w", err)
	}

	targetRepoSpec := ocireg.NewRepositorySpec(obj.Spec.Destination.URL, nil)
	target, err := octx.RepositoryForSpec(targetRepoSpec)
	if err != nil {
		panic(err)
	}
	defer source.Close()

	handler, err := standard.New(
		standard.Recursive(true),
		standard.ResourcesByValue(true),
		standard.Overwrite(true),
		standard.Resolver(source),
		standard.Resolver(target),
		// if additional resolvers are required they could be added here...
	)
	if err != nil {
		return fmt.Errorf("failed to construct target handler: %w", err)
	}
	if err := transfer.TransferVersion(
		nil,
		transfer.TransportClosure{},
		sourceComponentVersion,
		target,
		handler,
	); err != nil {
		return fmt.Errorf("failed to transfer version to destination repository: %w", err)
	}

	return nil
}

// maybeConfigureAccessCredentials configures access credentials if needed for a source/destination repository.
func (c *Client) maybeConfigureAccessCredentials(ctx context.Context, ocmCtx ocm.Context, repository v1alpha1.OCMRepository, namespace string) error {
	// If there are no credentials, this call is a no-op.
	if repository.SecretRef == nil {
		return nil
	}

	logger := log.FromContext(ctx)

	if err := csdk.ConfigureCredentials(ctx, ocmCtx, c.client, repository.URL, repository.SecretRef.Name, namespace); err != nil {
		logger.V(4).Error(err, "failed to find destination credentials")

		// we don't ignore not found errors
		return fmt.Errorf("failed to configure credentials for component: %w", err)
	}

	logger.V(4).Info("credentials configured")

	return nil
}

func (c *Client) maybeConfigureServiceAccountAccess(ctx context.Context, octx ocm.Context, obj *v1alpha1.ComponentSubscription) error {
	logger := log.FromContext(ctx)

	logger.V(4).Info("configuring service account credentials")

	if obj.Spec.ServiceAccountName == "" {
		return nil
	}

	account := &corev1.ServiceAccount{}
	if err := c.client.Get(ctx, types.NamespacedName{
		Name:      obj.Spec.ServiceAccountName,
		Namespace: obj.Namespace,
	}, account); err != nil {
		return fmt.Errorf("failed to fetch service account: %w", err)
	}

	logger.V(4).Info("got service account", "name", account.GetName())

	for _, imagePullSecret := range account.ImagePullSecrets {
		secret := &corev1.Secret{}

		if err := c.client.Get(ctx, types.NamespacedName{
			Name:      imagePullSecret.Name,
			Namespace: obj.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get image pull secret: %w", err)
		}

		data, ok := secret.Data[".dockerconfigjson"]
		if !ok {
			return fmt.Errorf("failed to find .dockerconfigjson in secret %s", secret.Name)
		}

		repository := dockerconfig.NewRepositorySpecForConfig(data, true)

		if _, err := octx.CredentialsContext().RepositoryForSpec(repository); err != nil {
			return fmt.Errorf("failed to configure credentials for repository: %w", err)
		}
	}

	return nil
}

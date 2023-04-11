package ocm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClient_GetComponentVersion(t *testing.T) {
	fakeKubeClient := env.FakeKubeClient()
	ocmClient := NewClient(fakeKubeClient)
	component := "github.com/skarlso/ocm-demo-index"

	err := env.AddComponentVersionToRepository(Component{
		Name:    component,
		Version: "v0.0.1",
	})
	require.NoError(t, err)

	cs := &v1alpha1.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "default",
		},
		Spec: v1alpha1.ComponentSubscriptionSpec{
			Component: component,
			Semver:    "v0.0.1",
			Source: v1alpha1.OCMRepository{
				URL: env.repositoryURL,
			},
		},
	}

	cva, err := ocmClient.GetComponentVersion(context.Background(), cs, "v0.0.1")
	assert.NoError(t, err)
	assert.Equal(t, cs.Spec.Component, cva.GetName())
}

func TestClient_GetLatestValidComponentVersion(t *testing.T) {
	testCases := []struct {
		name             string
		componentVersion func(name string) *v1alpha1.ComponentSubscription
		setupComponents  func(name string) error
		expectedVersion  string
	}{
		{
			name: "semver constraint works for greater versions",
			componentVersion: func(name string) *v1alpha1.ComponentSubscription {
				return &v1alpha1.ComponentSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentSubscriptionSpec{
						Component: name,
						Semver:    ">v0.0.1",
						Source: v1alpha1.OCMRepository{
							URL: env.repositoryURL,
						},
					},
				}
			},
			setupComponents: func(name string) error {
				// v0.0.1 should not be chosen.
				for _, v := range []string{"v0.0.1", "v0.0.5"} {
					if err := env.AddComponentVersionToRepository(Component{
						Name:    name,
						Version: v,
					}); err != nil {
						return err
					}
				}
				return nil
			},
			expectedVersion: "v0.0.5",
		},
		{
			name: "semver is a concrete match with multiple versions",
			componentVersion: func(name string) *v1alpha1.ComponentSubscription {
				return &v1alpha1.ComponentSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentSubscriptionSpec{
						Component: name,
						Semver:    "v0.0.1",
						Source: v1alpha1.OCMRepository{
							URL: env.repositoryURL,
						},
					},
				}
			},
			setupComponents: func(name string) error {
				for _, v := range []string{"v0.0.1", "v0.0.2", "v0.0.3"} {
					if err := env.AddComponentVersionToRepository(Component{
						Name:    name,
						Version: v,
					}); err != nil {
						return err
					}
				}
				return nil
			},
			expectedVersion: "v0.0.1",
		},
		{
			name: "semver is in between available versions should return the one that's still matching instead of the latest available",
			componentVersion: func(name string) *v1alpha1.ComponentSubscription {
				return &v1alpha1.ComponentSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentSubscriptionSpec{
						Component: name,
						Semver:    "<=v0.0.2",
						Source: v1alpha1.OCMRepository{
							URL: env.repositoryURL,
						},
					},
				}
			},
			setupComponents: func(name string) error {
				for _, v := range []string{"v0.0.1", "v0.0.2", "v0.0.3"} {
					if err := env.AddComponentVersionToRepository(Component{
						Name:    name,
						Version: v,
					}); err != nil {
						return err
					}
				}
				return nil
			},
			expectedVersion: "v0.0.2",
		},
		{
			name: "using = should still work as expected",
			componentVersion: func(name string) *v1alpha1.ComponentSubscription {
				return &v1alpha1.ComponentSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentSubscriptionSpec{
						Component: name,
						Semver:    "=v0.0.1",
						Source: v1alpha1.OCMRepository{
							URL: env.repositoryURL,
						},
					},
				}
			},
			setupComponents: func(name string) error {
				for _, v := range []string{"v0.0.1", "v0.0.2"} {
					if err := env.AddComponentVersionToRepository(Component{
						Name:    name,
						Version: v,
					}); err != nil {
						return err
					}
				}
				return nil
			},
			expectedVersion: "v0.0.1",
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeKubeClient := env.FakeKubeClient()
			ocmClient := NewClient(fakeKubeClient)
			component := "github.com/skarlso/ocm-demo-index"

			err := tt.setupComponents(component)
			require.NoError(t, err)
			cv := tt.componentVersion(component)

			latest, err := ocmClient.GetLatestSourceComponentVersion(context.Background(), cv)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedVersion, latest)
		})
	}
}

func TestClient_VerifyComponent(t *testing.T) {
	publicKey1, err := os.ReadFile(filepath.Join("testdata", "public1_key.pem"))
	require.NoError(t, err)
	privateKey, err := os.ReadFile(filepath.Join("testdata", "private_key.pem"))
	require.NoError(t, err)

	secretName := "sign-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			Signature: publicKey1,
		},
	}
	fakeKubeClient := env.FakeKubeClient(WithObjets(secret))
	ocmClient := NewClient(fakeKubeClient)
	component := "github.com/skarlso/ocm-demo-index"

	err = env.AddComponentVersionToRepository(Component{
		Name:    component,
		Version: "v0.0.1",
		Sign: &Sign{
			Name: Signature,
			Key:  privateKey,
		},
	})
	require.NoError(t, err)

	cv := &v1alpha1.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "default",
		},
		Spec: v1alpha1.ComponentSubscriptionSpec{
			Component: component,
			Source: v1alpha1.OCMRepository{
				URL: env.repositoryURL,
			},
			Verify: []v1alpha1.Signature{
				{
					Name: Signature,
					PublicKey: v1alpha1.SecretRef{
						SecretRef: meta.LocalObjectReference{
							Name: secretName,
						},
					},
				},
			},
		},
	}

	verified, err := ocmClient.VerifySourceComponent(context.Background(), cv, "v0.0.1")
	assert.NoError(t, err)
	assert.True(t, verified, "verified should have been true, but it did not")
}

func TestClient_VerifyComponentDifferentPublicKey(t *testing.T) {
	publicKey2, err := os.ReadFile(filepath.Join("testdata", "public2_key.pem"))
	require.NoError(t, err)
	privateKey, err := os.ReadFile(filepath.Join("testdata", "private_key.pem"))
	require.NoError(t, err)

	secretName := "sign-secret-2"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			Signature: publicKey2,
		},
	}
	fakeKubeClient := env.FakeKubeClient(WithObjets(secret))
	ocmClient := NewClient(fakeKubeClient)
	component := "github.com/skarlso/ocm-demo-index"

	err = env.AddComponentVersionToRepository(Component{
		Name:    component,
		Version: "v0.0.1",
		Sign: &Sign{
			Name: Signature,
			Key:  privateKey,
		},
	})
	require.NoError(t, err)

	cv := &v1alpha1.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "default",
		},
		Spec: v1alpha1.ComponentSubscriptionSpec{
			Component: component,
			Source: v1alpha1.OCMRepository{
				URL: env.repositoryURL,
			},
			Verify: []v1alpha1.Signature{
				{
					Name: Signature,
					PublicKey: v1alpha1.SecretRef{
						SecretRef: meta.LocalObjectReference{
							Name: secretName,
						},
					},
				},
			},
		},
	}

	verified, err := ocmClient.VerifySourceComponent(context.Background(), cv, "v0.0.1")
	require.Error(t, err)
	assert.False(t, verified, "verified should have been false, but it did not")
}

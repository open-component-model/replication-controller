package ocm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-component-model/ocm/pkg/contexts/credentials/cpi"
	"github.com/open-component-model/ocm/pkg/contexts/oci/identity"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
)

func TestClient_GetComponentVersion(t *testing.T) {
	testCases := []struct {
		name         string
		subscription func(component string, objs *[]client.Object) *v1alpha1.ComponentSubscription
	}{
		{
			name: "plain component access",
			subscription: func(component string, objs *[]client.Object) *v1alpha1.ComponentSubscription {
				return &v1alpha1.ComponentSubscription{
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
			},
		},
		{
			name: "component access with secret ref",
			subscription: func(component string, objs *[]client.Object) *v1alpha1.ComponentSubscription {
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
							SecretRef: &meta.LocalObjectReference{
								Name: "test-name-secret",
							},
						},
					},
				}
				testSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"token": []byte("token"),
					},
					Type: corev1.SecretTypeOpaque,
				}

				*objs = append(*objs, cs, testSecret)

				return cs
			},
		},
		{
			name: "component access with service account and image pull secret",
			subscription: func(component string, objs *[]client.Object) *v1alpha1.ComponentSubscription {
				cs := &v1alpha1.ComponentSubscription{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentSubscriptionSpec{
						Component:          component,
						Semver:             "v0.0.1",
						ServiceAccountName: "test-service-account",
						Source: v1alpha1.OCMRepository{
							URL: env.repositoryURL,
						},
					},
				}
				serviceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service-account",
						Namespace: "default",
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: "test-name-secret",
						},
					},
				}
				testSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-name-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						".dockerconfigjson": []byte(`{
  "auths": {
    "ghcr.io": {
      "username": "skarlso",
      "password": "password",
      "auth": "c2thcmxzbzpwYXNzd29yZAo="
    }
  }
}`),
					},
					Type: corev1.SecretTypeDockerConfigJson,
				}

				*objs = append(*objs, cs, testSecret, serviceAccount)

				return cs
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			objs := make([]client.Object, 0)
			component := "github.com/skarlso/ocm-demo-index"
			cs := tt.subscription(component, &objs)
			fakeKubeClient := env.FakeKubeClient(WithObjets(objs...))

			ocmClient := NewClient(fakeKubeClient)
			err := env.AddComponentVersionToRepository(Component{
				Name:    component,
				Version: "v0.0.1",
			})
			require.NoError(t, err)

			cva, err := ocmClient.GetComponentVersion(context.Background(), ocm.New(), cs, "v0.0.1")
			assert.NoError(t, err)

			assert.Equal(t, cs.Spec.Component, cva.GetName())
		})
	}
}

func TestClient_CreateAuthenticatedOCMContext(t *testing.T) {
	cs := &v1alpha1.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "default",
		},
		Spec: v1alpha1.ComponentSubscriptionSpec{
			Component: "github.com/skarlso/ocm-demo-index",
			Semver:    ">v0.0.1",
			Source: v1alpha1.OCMRepository{
				URL: env.repositoryURL,
			},
			ServiceAccountName: "test-service-account",
		},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-account",
			Namespace: "default",
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "test-name-secret",
			},
		},
	}
	testSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{
  "auths": {
    "ghcr.io": {
      "username": "skarlso",
      "password": "password",
      "auth": "c2thcmxzbzpwYXNzd29yZAo="
    }
  }
}`),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	fakeKubeClient := env.FakeKubeClient(WithObjets(cs, serviceAccount, testSecret))
	ocmClient := NewClient(fakeKubeClient)
	component := "github.com/skarlso/ocm-demo-index"

	err := env.AddComponentVersionToRepository(Component{
		Name:    component,
		Version: "v0.0.1",
	})
	require.NoError(t, err)

	octx, err := ocmClient.CreateAuthenticatedOCMContext(context.Background(), cs)
	require.NoError(t, err)

	id := cpi.ConsumerIdentity{
		identity.ID_TYPE:       identity.CONSUMER_TYPE,
		identity.ID_HOSTNAME:   "ghcr.io",
		identity.ID_PATHPREFIX: "skarlso",
	}
	creds, err := octx.CredentialsContext().GetCredentialsForConsumer(id)
	require.NoError(t, err)
	consumer, err := creds.Credentials(nil)
	require.NoError(t, err)

	assert.Equal(t, "password", consumer.Properties()["password"])
	assert.Equal(t, "skarlso", consumer.Properties()["username"])
	assert.Equal(t, "ghcr.io", consumer.Properties()["serverAddress"])
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

			latest, err := ocmClient.GetLatestSourceComponentVersion(context.Background(), ocm.New(), cv)
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

	verified, err := ocmClient.VerifySourceComponent(context.Background(), ocm.New(), cv, "v0.0.1")
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

	verified, err := ocmClient.VerifySourceComponent(context.Background(), ocm.New(), cv, "v0.0.1")
	require.Error(t, err)
	assert.False(t, verified, "verified should have been false, but it did not")
}

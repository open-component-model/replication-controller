package controllers

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	ocmdesc "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc"
	v1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	v1alpha12 "github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/open-component-model/replication-controller/pkg/ocm/fakes"
)

func TestComponentSubscriptionReconciler(t *testing.T) {
	testCases := []struct {
		name         string
		subscription func() *v1alpha12.ComponentSubscription
		setupMock    func(*fakes.MockFetcher)
		err          string
	}{
		{
			name: "reconcile function succeeds",
			subscription: func() *v1alpha12.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				return cv
			},
			setupMock: func(fakeOcm *fakes.MockFetcher) {
				root := &mockComponent{
					t: t,
					descriptor: &ocmdesc.ComponentDescriptor{
						ComponentSpec: ocmdesc.ComponentSpec{
							ObjectMeta: v1.ObjectMeta{
								Name:    "github.com/open-component-model/component",
								Version: "v0.0.1",
							},
							References: ocmdesc.References{
								{
									ElementMeta: ocmdesc.ElementMeta{
										Name:    "test-ref-1",
										Version: "v0.0.1",
									},
									ComponentName: "github.com/skarlso/embedded",
								},
							},
						},
					},
				}
				fakeOcm.GetComponentVersionReturnsForName(root.descriptor.ComponentSpec.Name, root, nil)
				fakeOcm.GetLatestComponentVersionReturns("v0.0.1", nil)
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cv := tt.subscription()
			client := env.FakeKubeClient(WithObjets(cv))
			fakeOcm := &fakes.MockFetcher{}
			tt.setupMock(fakeOcm)

			cvr := ComponentSubscriptionReconciler{
				Scheme:    env.scheme,
				Client:    client,
				OCMClient: fakeOcm,
			}

			_, err := cvr.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      cv.Name,
					Namespace: cv.Namespace,
				},
			})
			require.NoError(t, err)

			t.Log("verifying updated object status")
			err = client.Get(context.Background(), types.NamespacedName{
				Name:      cv.Name,
				Namespace: cv.Namespace,
			}, cv)

			if tt.err != "" {
				require.NoError(t, err)
				assert.Equal(t, cv.Status.LatestVersion, "v0.0.1")
				assert.True(t, conditions.IsTrue(cv, meta.ReadyCondition))
			}
		})
	}

}

type mockComponent struct {
	descriptor *ocmdesc.ComponentDescriptor
	ocm.ComponentVersionAccess
	t *testing.T
}

func (m *mockComponent) GetName() string {
	return m.descriptor.Name
}

func (m *mockComponent) GetDescriptor() *ocmdesc.ComponentDescriptor {
	return m.descriptor
}

func (m *mockComponent) Close() error {
	return nil
}

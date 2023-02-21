package controllers

import (
	"context"
	"errors"
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
		verifyMock   func(fetcher *fakes.MockFetcher) bool
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
			verifyMock: func(fetcher *fakes.MockFetcher) bool {
				args := fetcher.TransferComponentCallingArgumentsOnCall(0)
				obj, version := args[0], args[2]
				cv := obj.(*v1alpha12.ComponentSubscription)
				return cv.Status.LatestVersion == "v0.0.1" && version.(string) == "v0.0.1"
			},
		},
		{
			name: "reconciling doesn't happen if version was already reconciled",
			subscription: func() *v1alpha12.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				cv.Status.LatestVersion = "v0.0.1"
				cv.Status.ReplicatedVersion = "v0.0.1"
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
			verifyMock: func(fetcher *fakes.MockFetcher) bool {
				return fetcher.TransferComponentWasNotCalled()
			},
		},
		{
			name: "reconcile fails if transfer version fails",
			subscription: func() *v1alpha12.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				return cv
			},
			err: "failed to transfer components: nope",
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
				fakeOcm.TransferComponentReturns(errors.New("nope"))
			},
			verifyMock: func(fetcher *fakes.MockFetcher) bool {
				args := fetcher.TransferComponentCallingArgumentsOnCall(0)
				obj, version := args[0], args[2]
				cv := obj.(*v1alpha12.ComponentSubscription)
				return cv.Status.LatestVersion == "v0.0.1" && version.(string) == "v0.0.1"
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
			t.Log("verifying updated object status")
			if tt.err == "" {
				err = client.Get(context.Background(), types.NamespacedName{
					Name:      cv.Name,
					Namespace: cv.Namespace,
				}, cv)
				require.NoError(t, err)
				assert.Equal(t, cv.Status.LatestVersion, "v0.0.1")
				assert.True(t, conditions.IsTrue(cv, meta.ReadyCondition))
			} else {
				assert.EqualError(t, err, tt.err)
			}

			assert.True(t, tt.verifyMock(fakeOcm))
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

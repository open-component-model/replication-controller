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
	"github.com/open-component-model/replication-controller/pkg/sign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/open-component-model/replication-controller/pkg/ocm/fakes"
)

func TestComponentSubscriptionReconciler(t *testing.T) {
	testCases := []struct {
		name         string
		subscription func() *v1alpha1.ComponentSubscription
		setupMock    func(*fakes.MockFetcher)
		verifyMock   func(fetcher *fakes.MockFetcher) bool
		mpasEnabled  bool
		err          string
	}{
		{
			name: "reconcile function succeeds",
			subscription: func() *v1alpha1.ComponentSubscription {
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
									ComponentName: "github.com/open-component-model/embedded",
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
				obj := args[0]
				cv := obj.(*v1alpha1.ComponentSubscription)
				return cv.Status.LastAttemptedVersion == "v0.0.1"
			},
		},
		{
			name: "mpas enabled component is signed",
			subscription: func() *v1alpha1.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				return cv
			},
			setupMock: func(fakeOcm *fakes.MockFetcher) {
				root := &mockComponent{
					resourceAccess: []ocm.ResourceAccess{
						nil,
					},
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
									ComponentName: "github.com/open-component-model/embedded",
								},
							},
						},
					},
				}
				_, pub, err := sign.GenerateSigningKeyPEMPair()
				require.NoError(t, err)
				fakeOcm.GetComponentVersionReturnsForName(root.descriptor.ComponentSpec.Name, root, nil)
				fakeOcm.GetLatestComponentVersionReturns("v0.0.1", nil)
				fakeOcm.SignDestinationComponentReturns(pub, nil)
			},
			verifyMock: func(fetcher *fakes.MockFetcher) bool {
				args := fetcher.SignDestinationComponentCallingArgumentsOnCall(0)
				name := args[0]
				return name == "github.com/open-component-model/component"
			},
			mpasEnabled: true,
		},
		{
			name: "no transfer is called if destination is left empty",
			subscription: func() *v1alpha1.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				cv.Spec.Destination = nil
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
									ComponentName: "github.com/open-component-model/embedded",
								},
							},
						},
					},
				}
				fakeOcm.GetComponentVersionReturnsForName(root.descriptor.ComponentSpec.Name, root, nil)
				fakeOcm.GetLatestComponentVersionReturns("v0.0.1", nil)
			},
			verifyMock: func(fetcher *fakes.MockFetcher) bool {
				return fetcher.TransferComponentWasNotCalled() && fetcher.SignDestinationComponentNotCalled()
			},
		},
		{
			name: "reconciling doesn't happen if version was already reconciled",
			subscription: func() *v1alpha1.ComponentSubscription {
				cv := DefaultComponentSubscription.DeepCopy()
				cv.Status.LastAttemptedVersion = "v0.0.1"
				cv.Status.LastAppliedVersion = "v0.0.1"
				cv.Status.ReplicatedRepositoryURL = "https://destination.com"
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
									ComponentName: "github.com/open-component-model/embedded",
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
			subscription: func() *v1alpha1.ComponentSubscription {
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
									ComponentName: "github.com/open-component-model/embedded",
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
				obj := args[0]
				cv := obj.(*v1alpha1.ComponentSubscription)
				return cv.Status.LastAttemptedVersion == "v0.0.1"
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cv := tt.subscription()
			client := env.FakeKubeClient(WithObjets(cv))
			fakeOcm := &fakes.MockFetcher{}
			recorder := &record.FakeRecorder{
				Events:        make(chan string, 32),
				IncludeObject: true,
			}
			tt.setupMock(fakeOcm)

			cvr := ComponentSubscriptionReconciler{
				Scheme:        env.scheme,
				Client:        client,
				OCMClient:     fakeOcm,
				EventRecorder: recorder,
				MpasEnabled:   tt.mpasEnabled,
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
				assert.Equal(t, cv.Status.LastAttemptedVersion, "v0.0.1")
				assert.True(t, conditions.IsTrue(cv, meta.ReadyCondition))
				if cv.Spec.Destination != nil {
					assert.Equal(t, cv.Spec.Destination.URL, cv.Status.ReplicatedRepositoryURL)
				} else {
					assert.Equal(t, cv.Spec.Source.URL, cv.Status.ReplicatedRepositoryURL)
				}

				if tt.mpasEnabled {
					assert.NotEmpty(t, cv.Status.Signature)
					sigName := cv.Status.Signature[0].Name
					sigKey := cv.Status.Signature[0].PublicKey.Value
					assert.Equal(t, v1alpha1.InternalSignatureName, sigName)
					assert.NotEmpty(t, sigKey)
				}
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
	resourceAccess []ocm.ResourceAccess
	t              *testing.T
}

func (m *mockComponent) GetName() string {
	return m.descriptor.Name
}

func (m *mockComponent) GetDescriptor() *ocmdesc.ComponentDescriptor {
	return m.descriptor
}

func (m *mockComponent) GetResourcesByResourceSelectors(selectors ...ocmdesc.ResourceSelector) ([]ocm.ResourceAccess, error) {
	return m.resourceAccess, nil
}

func (m *mockComponent) Close() error {
	return nil
}

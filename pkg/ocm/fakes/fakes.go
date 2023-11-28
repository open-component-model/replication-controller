// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package fakes

import (
	"context"

	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	ocm2 "github.com/open-component-model/replication-controller/pkg/ocm"

	"github.com/open-component-model/replication-controller/api/v1alpha1"
)

// MockFetcher mocks OCM client. Sadly, no generated code can be used, because none of them understand
// not importing type aliased names that OCM uses. Meaning, external types request internally aliased
// resources and the mock does not compile.
// I.e.: counterfeiter: https://github.com/maxbrunsfeld/counterfeiter/issues/174
type MockFetcher struct {
	getComponentVersionMap              map[string]ocm.ComponentVersionAccess
	getComponentVersionErr              error
	getComponentVersionCalledWith       [][]any
	verifySourceComponentErr            error
	verifySourceComponentVerified       bool
	verifySourceComponentCalledWith     [][]any
	getLatestComponentVersionVersion    string
	getLatestComponentVersionErr        error
	getLatestComponentVersionCalledWith [][]any
	transferComponentVersionErr         error
	transferComponentVersionCalledWith  [][]any
	signDestinationComponentCalledWith  [][]any
}

var _ ocm2.Contract = &MockFetcher{}

func (m *MockFetcher) SignDestinationComponent(_ context.Context, component ocm.ComponentVersionAccess) ([]byte, error) {
	m.signDestinationComponentCalledWith = append(m.signDestinationComponentCalledWith, []any{component.GetName()})
	return nil, nil
}
func (m *MockFetcher) SignDestinationComponentNotCalled() bool {
	return len(m.signDestinationComponentCalledWith) == 0
}

func (m *MockFetcher) SignDestinationComponentCallingArgumentsOnCall(i int) []any {
	if i > len(m.signDestinationComponentCalledWith) {
		return nil
	}

	return m.signDestinationComponentCalledWith[i]
}

func (m *MockFetcher) CreateAuthenticatedOCMContext(ctx context.Context, obj *v1alpha1.ComponentSubscription) (ocm.Context, error) {
	return ocm.New(), nil
}

func (m *MockFetcher) GetComponentVersion(ctx context.Context, octx ocm.Context, obj *v1alpha1.ComponentSubscription, version string) (ocm.ComponentVersionAccess, error) {
	m.getComponentVersionCalledWith = append(m.getComponentVersionCalledWith, []any{obj, version})
	return m.getComponentVersionMap[obj.Spec.Component], m.getComponentVersionErr
}

func (m *MockFetcher) GetComponentVersionReturnsForName(name string, cva ocm.ComponentVersionAccess, err error) {
	if m.getComponentVersionMap == nil {
		m.getComponentVersionMap = make(map[string]ocm.ComponentVersionAccess)
	}
	m.getComponentVersionMap[name] = cva
	m.getComponentVersionErr = err
}

func (m *MockFetcher) GetComponentVersionCallingArgumentsOnCall(i int) []any {
	return m.getComponentVersionCalledWith[i]
}

func (m *MockFetcher) GetComponentVersionWasNotCalled() bool {
	return len(m.getComponentVersionCalledWith) == 0
}

func (m *MockFetcher) VerifySourceComponent(ctx context.Context, octx ocm.Context, obj *v1alpha1.ComponentSubscription, version string) (bool, error) {
	m.verifySourceComponentCalledWith = append(m.verifySourceComponentCalledWith, []any{obj, version})
	return m.verifySourceComponentVerified, m.verifySourceComponentErr
}

func (m *MockFetcher) VerifySourceComponentReturns(verified bool, err error) {
	m.verifySourceComponentVerified = verified
	m.verifySourceComponentErr = err
}

func (m *MockFetcher) VerifySourceComponentCallingArgumentsOnCall(i int) []any {
	return m.verifySourceComponentCalledWith[i]
}

func (m *MockFetcher) VerifySourceComponentWasNotCalled() bool {
	return len(m.verifySourceComponentCalledWith) == 0
}

func (m *MockFetcher) GetLatestSourceComponentVersion(ctx context.Context, octx ocm.Context, obj *v1alpha1.ComponentSubscription) (string, error) {
	m.getComponentVersionCalledWith = append(m.getComponentVersionCalledWith, []any{obj})
	return m.getLatestComponentVersionVersion, m.getLatestComponentVersionErr
}

func (m *MockFetcher) GetLatestComponentVersionReturns(version string, err error) {
	m.getLatestComponentVersionVersion = version
	m.getLatestComponentVersionErr = err
}

func (m *MockFetcher) GetLatestComponentVersionCallingArgumentsOnCall(i int) []any {
	return m.getLatestComponentVersionCalledWith[i]
}

func (m *MockFetcher) GetLatestComponentVersionWasNotCalled() bool {
	return len(m.getLatestComponentVersionCalledWith) == 0
}

func (m *MockFetcher) TransferComponent(ctx context.Context, octx ocm.Context, obj *v1alpha1.ComponentSubscription, sourceComponentVersion ocm.ComponentVersionAccess, version string) error {
	m.transferComponentVersionCalledWith = append(m.transferComponentVersionCalledWith, []any{obj, sourceComponentVersion, version})
	return m.transferComponentVersionErr
}

func (m *MockFetcher) TransferComponentReturns(err error) {
	m.transferComponentVersionErr = err
}

func (m *MockFetcher) TransferComponentWasNotCalled() bool {
	return len(m.transferComponentVersionCalledWith) == 0
}

func (m *MockFetcher) TransferComponentCallingArgumentsOnCall(i int) []any {
	return m.transferComponentVersionCalledWith[i]
}

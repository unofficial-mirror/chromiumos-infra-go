// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
// Code generated by MockGen. DO NOT EDIT.
// Source: internal/checkout/checkout.go

// Package mock_checkout is a generated GoMock package.
package mock_checkout

import (
	gomock "github.com/golang/mock/gomock"
	repo "go.chromium.org/chromiumos/infra/go/internal/repo"
	reflect "reflect"
	regexp "regexp"
)

// MockCheckout is a mock of Checkout interface
type MockCheckout struct {
	ctrl     *gomock.Controller
	recorder *MockCheckoutMockRecorder
}

// MockCheckoutMockRecorder is the mock recorder for MockCheckout
type MockCheckoutMockRecorder struct {
	mock *MockCheckout
}

// NewMockCheckout creates a new mock instance
func NewMockCheckout(ctrl *gomock.Controller) *MockCheckout {
	mock := &MockCheckout{ctrl: ctrl}
	mock.recorder = &MockCheckoutMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockCheckout) EXPECT() *MockCheckoutMockRecorder {
	return m.recorder
}

// Initialize mocks base method
func (m *MockCheckout) Initialize(root, manifestUrl string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Initialize", root, manifestUrl)
	ret0, _ := ret[0].(error)
	return ret0
}

// Initialize indicates an expected call of Initialize
func (mr *MockCheckoutMockRecorder) Initialize(root, manifestUrl interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Initialize", reflect.TypeOf((*MockCheckout)(nil).Initialize), root, manifestUrl)
}

// Manifest mocks base method
func (m *MockCheckout) Manifest() repo.Manifest {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Manifest")
	ret0, _ := ret[0].(repo.Manifest)
	return ret0
}

// Manifest indicates an expected call of Manifest
func (mr *MockCheckoutMockRecorder) Manifest() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Manifest", reflect.TypeOf((*MockCheckout)(nil).Manifest))
}

// SetRepoToolPath mocks base method
func (m *MockCheckout) SetRepoToolPath(path string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetRepoToolPath", path)
}

// SetRepoToolPath indicates an expected call of SetRepoToolPath
func (mr *MockCheckoutMockRecorder) SetRepoToolPath(path interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetRepoToolPath", reflect.TypeOf((*MockCheckout)(nil).SetRepoToolPath), path)
}

// SyncToManifest mocks base method
func (m *MockCheckout) SyncToManifest(path string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SyncToManifest", path)
	ret0, _ := ret[0].(error)
	return ret0
}

// SyncToManifest indicates an expected call of SyncToManifest
func (mr *MockCheckoutMockRecorder) SyncToManifest(path interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SyncToManifest", reflect.TypeOf((*MockCheckout)(nil).SyncToManifest), path)
}

// ReadVersion mocks base method
func (m *MockCheckout) ReadVersion() repo.VersionInfo {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadVersion")
	ret0, _ := ret[0].(repo.VersionInfo)
	return ret0
}

// ReadVersion indicates an expected call of ReadVersion
func (mr *MockCheckoutMockRecorder) ReadVersion() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadVersion", reflect.TypeOf((*MockCheckout)(nil).ReadVersion))
}

// AbsolutePath mocks base method
func (m *MockCheckout) AbsolutePath(args ...string) string {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "AbsolutePath", varargs...)
	ret0, _ := ret[0].(string)
	return ret0
}

// AbsolutePath indicates an expected call of AbsolutePath
func (mr *MockCheckoutMockRecorder) AbsolutePath(args ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AbsolutePath", reflect.TypeOf((*MockCheckout)(nil).AbsolutePath), args...)
}

// AbsoluteProjectPath mocks base method
func (m *MockCheckout) AbsoluteProjectPath(project repo.Project, args ...string) string {
	m.ctrl.T.Helper()
	varargs := []interface{}{project}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "AbsoluteProjectPath", varargs...)
	ret0, _ := ret[0].(string)
	return ret0
}

// AbsoluteProjectPath indicates an expected call of AbsoluteProjectPath
func (mr *MockCheckoutMockRecorder) AbsoluteProjectPath(project interface{}, args ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{project}, args...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AbsoluteProjectPath", reflect.TypeOf((*MockCheckout)(nil).AbsoluteProjectPath), varargs...)
}

// BranchExists mocks base method
func (m *MockCheckout) BranchExists(project repo.Project, pattern *regexp.Regexp) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BranchExists", project, pattern)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BranchExists indicates an expected call of BranchExists
func (mr *MockCheckoutMockRecorder) BranchExists(project, pattern interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BranchExists", reflect.TypeOf((*MockCheckout)(nil).BranchExists), project, pattern)
}

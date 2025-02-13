// Code generated by MockGen. DO NOT EDIT.
// Source: pagerduty.go
//
// Generated by this command:
//
//	mockgen -package mock_publisher -destination ../mock/publisher/pagerduty.go -source pagerduty.go -mock_names pagerDutyClient=Mock_pagerDutyClient pagerDutyClient
//

// Package mock_publisher is a generated GoMock package.
package mock_publisher

import (
	context "context"
	reflect "reflect"

	pagerduty "github.com/PagerDuty/go-pagerduty"
	gomock "go.uber.org/mock/gomock"
)

// Mock_pagerDutyClient is a mock of pagerDutyClient interface.
type Mock_pagerDutyClient struct {
	ctrl     *gomock.Controller
	recorder *Mock_pagerDutyClientMockRecorder
	isgomock struct{}
}

// Mock_pagerDutyClientMockRecorder is the mock recorder for Mock_pagerDutyClient.
type Mock_pagerDutyClientMockRecorder struct {
	mock *Mock_pagerDutyClient
}

// NewMock_pagerDutyClient creates a new mock instance.
func NewMock_pagerDutyClient(ctrl *gomock.Controller) *Mock_pagerDutyClient {
	mock := &Mock_pagerDutyClient{ctrl: ctrl}
	mock.recorder = &Mock_pagerDutyClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *Mock_pagerDutyClient) EXPECT() *Mock_pagerDutyClientMockRecorder {
	return m.recorder
}

// ManageEventWithContext mocks base method.
func (m *Mock_pagerDutyClient) ManageEventWithContext(arg0 context.Context, arg1 *pagerduty.V2Event) (*pagerduty.V2EventResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ManageEventWithContext", arg0, arg1)
	ret0, _ := ret[0].(*pagerduty.V2EventResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ManageEventWithContext indicates an expected call of ManageEventWithContext.
func (mr *Mock_pagerDutyClientMockRecorder) ManageEventWithContext(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ManageEventWithContext", reflect.TypeOf((*Mock_pagerDutyClient)(nil).ManageEventWithContext), arg0, arg1)
}

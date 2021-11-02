// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ScaleProcess struct {
	Stub        func(context.Context, client.Client, string, repositories.ProcessScale) (repositories.ProcessRecord, error)
	mutex       sync.RWMutex
	argsForCall []struct {
		arg1 context.Context
		arg2 client.Client
		arg3 string
		arg4 repositories.ProcessScale
	}
	returns struct {
		result1 repositories.ProcessRecord
		result2 error
	}
	returnsOnCall map[int]struct {
		result1 repositories.ProcessRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ScaleProcess) Spy(arg1 context.Context, arg2 client.Client, arg3 string, arg4 repositories.ProcessScale) (repositories.ProcessRecord, error) {
	fake.mutex.Lock()
	ret, specificReturn := fake.returnsOnCall[len(fake.argsForCall)]
	fake.argsForCall = append(fake.argsForCall, struct {
		arg1 context.Context
		arg2 client.Client
		arg3 string
		arg4 repositories.ProcessScale
	}{arg1, arg2, arg3, arg4})
	stub := fake.Stub
	returns := fake.returns
	fake.recordInvocation("ScaleProcess", []interface{}{arg1, arg2, arg3, arg4})
	fake.mutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return returns.result1, returns.result2
}

func (fake *ScaleProcess) CallCount() int {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return len(fake.argsForCall)
}

func (fake *ScaleProcess) Calls(stub func(context.Context, client.Client, string, repositories.ProcessScale) (repositories.ProcessRecord, error)) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = stub
}

func (fake *ScaleProcess) ArgsForCall(i int) (context.Context, client.Client, string, repositories.ProcessScale) {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return fake.argsForCall[i].arg1, fake.argsForCall[i].arg2, fake.argsForCall[i].arg3, fake.argsForCall[i].arg4
}

func (fake *ScaleProcess) Returns(result1 repositories.ProcessRecord, result2 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	fake.returns = struct {
		result1 repositories.ProcessRecord
		result2 error
	}{result1, result2}
}

func (fake *ScaleProcess) ReturnsOnCall(i int, result1 repositories.ProcessRecord, result2 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	if fake.returnsOnCall == nil {
		fake.returnsOnCall = make(map[int]struct {
			result1 repositories.ProcessRecord
			result2 error
		})
	}
	fake.returnsOnCall[i] = struct {
		result1 repositories.ProcessRecord
		result2 error
	}{result1, result2}
}

func (fake *ScaleProcess) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ScaleProcess) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ apis.ScaleProcess = new(ScaleProcess).Spy

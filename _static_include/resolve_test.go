package vk

import (
	"strings"
	"sync"
	"testing"
)

// Tests of command resolution (see resolveSlow in static_common.go) against the fake library, which exports
// vkGetInstanceProcAddr and a command that is reachable only through it, as an extension command is through the loader.

// expectPanic runs f and fails the test unless it panics with a message containing want.
func expectPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("expected a panic mentioning %q, but the call returned", want)
		} else if msg, ok := r.(string); !ok || !strings.Contains(msg, want) {
			t.Errorf("panic = %v, want a message mentioning %q", r, want)
		}
	}()
	f()
}

func newFakeInstance(t *testing.T) Instance {
	t.Helper()
	instance, r := CreateInstance(&InstanceCreateInfo{}, nil)
	if r != SUCCESS {
		t.Fatalf("CreateInstance: %v", r)
	}
	return instance
}

func callStencilReference() {
	CmdSetStencilReference(CommandBuffer(0x1234), StencilFaceFlags(2), 7)
}

func TestResolveThroughGetInstanceProcAddr(t *testing.T) {
	requireFakeLibrary(t)

	// With no instance, a command that is not an exported symbol cannot be found, and says so rather than crashing
	expectPanic(t, "vkCmdSetStencilReference", callStencilReference)

	instance := newFakeInstance(t)

	vkCall_i(cmdReset, 0)
	callStencilReference()
	for i, want := range []uintptr{0x1234, 2, 7} {
		if got := gotInt(uintptr(i)); got != want {
			t.Errorf("integer arg %d: got %#x, want %#x", i, got, want)
		}
	}

	// The pointer belongs to the instance and is discarded with it
	DestroyInstance(instance, nil)
	expectPanic(t, "vkCmdSetStencilReference", callStencilReference)
}

func TestResolveWithMultipleInstances(t *testing.T) {
	requireFakeLibrary(t)

	first, second := newFakeInstance(t), newFakeInstance(t)
	if first == second {
		t.Fatal("the fake library should return distinct instance handles")
	}

	// Destroying one instance discards cached pointers, but the other instance can still resolve them
	DestroyInstance(first, nil)
	callStencilReference()

	DestroyInstance(second, nil)
	expectPanic(t, "vkCmdSetStencilReference", callStencilReference)
}

// TestConcurrentResolve is most useful under the race detector (go test -race): many goroutines resolve the same command
// for the first time at once.
func TestConcurrentResolve(t *testing.T) {
	requireFakeLibrary(t)

	instance := newFakeInstance(t)
	defer DestroyInstance(instance, nil)

	wg := &sync.WaitGroup{}
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callStencilReference()
		}()
	}
	wg.Wait()
}

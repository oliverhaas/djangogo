package djangotest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
)

// fakeT is a djangotest.TB that records failures instead of failing the
// running test, so the tests can assert that an assertion fails and with which
// message. It plays the role of assertRaises(AssertionError) in Django's tests
// of its own assertions.
type fakeT struct {
	messages []string
}

// fatalSignal is the panic value fakeT.Fatalf uses to stop the function under
// test, as testing.T.Fatalf stops the test goroutine.
type fatalSignal struct{}

func (f *fakeT) Helper() {}

func (f *fakeT) Errorf(format string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
	panic(fatalSignal{})
}

// failures runs fn with a recording TB and returns the failure messages it
// produced. A Fatalf inside fn stops fn, as it would stop a real test.
func failures(fn func(t djangotest.TB)) (messages []string) {
	ft := &fakeT{}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSignal); !ok {
				panic(r)
			}
		}
		messages = ft.messages
	}()
	fn(ft)
	return ft.messages
}

// mustFail asserts that fn reports a failure whose message contains want (any
// failure when want is empty).
func mustFail(t *testing.T, want string, fn func(t djangotest.TB)) {
	t.Helper()
	msgs := failures(fn)
	if len(msgs) == 0 {
		t.Errorf("assertion passed, want a failure containing %q", want)
		return
	}
	if joined := strings.Join(msgs, "; "); !strings.Contains(joined, want) {
		t.Errorf("failure message %q does not contain %q", joined, want)
	}
}

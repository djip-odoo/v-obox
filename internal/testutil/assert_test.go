package testutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

// mockTB is a lightweight testing.TB implementation for testing assertion failures
type mockTB struct {
	testing.TB
	failed   bool
	fatalMsg string
}

func (m *mockTB) Helper() {}
func (m *mockTB) Fatalf(format string, args ...interface{}) {
	m.failed = true
	m.fatalMsg = formatMessage(format, args...)
}
func (m *mockTB) Fatal(args ...interface{}) {
	m.failed = true
	m.fatalMsg = fmt.Sprint(args...)
}
func (m *mockTB) Cleanup(f func()) {}

func TestAssertions_Success(t *testing.T) {
	mt := &mockTB{}

	ExpectedTrue(mt, true, "should pass")
	AssertTrue(mt, true)
	ExpectedFalse(mt, false, "should pass")
	AssertFalse(mt, false)

	ExpectedEqual(mt, 42, 42, "numbers equal")
	AssertEqual(mt, "hello", "hello")
	ExpectedNotEqual(mt, 1, 2)
	AssertNotEqual(mt, "a", "b")

	ExpectedBytesEqual(mt, []byte{1, 2}, []byte{1, 2})
	AssertBytesEqual(mt, []byte{3}, []byte{3})

	ExpectedNoError(mt, nil)
	AssertNoError(mt, nil)

	testErr := errors.New("something went wrong")
	ExpectedError(mt, testErr)
	AssertError(mt, testErr)
	ExpectedErrorContains(mt, testErr, "went wrong")
	AssertErrorContains(mt, testErr, "something")

	ExpectedContains(mt, "hello world", "world")
	AssertContains(mt, "apple banana", "apple")
	ExpectedNotContains(mt, "hello world", "foo")
	AssertNotContains(mt, "apple banana", "orange")

	var nilPtr *int = nil
	ExpectedNil(mt, nilPtr)
	AssertNil(mt, nil)

	notNilVal := 10
	ExpectedNotNil(mt, &notNilVal)
	AssertNotNil(mt, "string")

	ExpectedLen(mt, []int{1, 2, 3}, 3)
	AssertLen(mt, []string{"a"}, 1)

	if mt.failed {
		t.Fatalf("Expected all valid assertions to pass, but mockTB failed with: %s", mt.fatalMsg)
	}
}

func TestAssertions_Failures(t *testing.T) {
	tests := []struct {
		name string
		fn   func(tb *mockTB)
	}{
		{"ExpectedTrue failure", func(tb *mockTB) { ExpectedTrue(tb, false) }},
		{"ExpectedFalse failure", func(tb *mockTB) { ExpectedFalse(tb, true) }},
		{"ExpectedEqual failure", func(tb *mockTB) { ExpectedEqual(tb, 1, 2) }},
		{"ExpectedNotEqual failure", func(tb *mockTB) { ExpectedNotEqual(tb, 5, 5) }},
		{"ExpectedBytesEqual failure", func(tb *mockTB) { ExpectedBytesEqual(tb, []byte{1}, []byte{2}) }},
		{"ExpectedNoError failure", func(tb *mockTB) { ExpectedNoError(tb, errors.New("fail")) }},
		{"ExpectedError failure", func(tb *mockTB) { ExpectedError(tb, nil) }},
		{"ExpectedErrorContains failure nil err", func(tb *mockTB) { ExpectedErrorContains(tb, nil, "sub") }},
		{"ExpectedErrorContains failure missing sub", func(tb *mockTB) { ExpectedErrorContains(tb, errors.New("err"), "missing") }},
		{"ExpectedContains failure", func(tb *mockTB) { ExpectedContains(tb, "abc", "xyz") }},
		{"ExpectedNotContains failure", func(tb *mockTB) { ExpectedNotContains(tb, "abc", "b") }},
		{"ExpectedNil failure", func(tb *mockTB) { ExpectedNil(tb, 123) }},
		{"ExpectedNotNil failure", func(tb *mockTB) { ExpectedNotNil(tb, nil) }},
		{"ExpectedLen failure", func(tb *mockTB) { ExpectedLen(tb, []int{1, 2}, 5) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mt := &mockTB{}
			tc.fn(mt)
			if !mt.failed {
				t.Errorf("Expected %s to trigger failure, but it passed", tc.name)
			}
		})
	}
}

func TestStartMockTCPServer(t *testing.T) {
	received := make(chan string, 1)
	ln, port, err := StartMockTCPServer(t, 0, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 16)
		n, _ := io.ReadAtLeast(conn, buf, 1)
		received <- string(buf[:n])
	})

	if err != nil {
		t.Skipf("TCP listeners unavailable in this environment: %v", err)
	}
	defer ln.Close()

	if port <= 0 {
		t.Errorf("Expected port > 0, got %d", port)
	}

	// Dial mock server and send bytes
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial mock TCP server: %v", err)
	}
	_, _ = conn.Write([]byte("PING"))
	_ = conn.Close()

	got := <-received
	if got != "PING" {
		t.Errorf("Received %q, want 'PING'", got)
	}
}

package testutil

import (
	"bytes"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
)

// formatMessage formats optional message arguments or returns a default fallback.
func formatMessage(defaultMsg string, msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 {
		return defaultMsg
	}
	if format, ok := msgAndArgs[0].(string); ok {
		if len(msgAndArgs) > 1 {
			return fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		return format
	}
	return fmt.Sprint(msgAndArgs...)
}

// ExpectedTrue asserts that the given condition is true.
func ExpectedTrue(t testing.TB, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if !condition {
		msg := formatMessage("expected condition to be true, got false", msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertTrue is an alias for ExpectedTrue.
func AssertTrue(t testing.TB, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedTrue(t, condition, msgAndArgs...)
}

// ExpectedFalse asserts that the given condition is false.
func ExpectedFalse(t testing.TB, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if condition {
		msg := formatMessage("expected condition to be false, got true", msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertFalse is an alias for ExpectedFalse.
func AssertFalse(t testing.TB, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedFalse(t, condition, msgAndArgs...)
}

// ExpectedEqual asserts that actual equals expected.
func ExpectedEqual[T comparable](t testing.TB, actual, expected T, msgAndArgs ...interface{}) {
	t.Helper()
	if actual != expected {
		msg := formatMessage(fmt.Sprintf("expected %v, got %v", expected, actual), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertEqual is an alias for ExpectedEqual.
func AssertEqual[T comparable](t testing.TB, actual, expected T, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedEqual(t, actual, expected, msgAndArgs...)
}

// ExpectedNotEqual asserts that actual is not equal to expected.
func ExpectedNotEqual[T comparable](t testing.TB, actual, expected T, msgAndArgs ...interface{}) {
	t.Helper()
	if actual == expected {
		msg := formatMessage(fmt.Sprintf("expected value not to equal %v", expected), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertNotEqual is an alias for ExpectedNotEqual.
func AssertNotEqual[T comparable](t testing.TB, actual, expected T, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedNotEqual(t, actual, expected, msgAndArgs...)
}

// ExpectedBytesEqual asserts that actual byte slice equals expected byte slice.
func ExpectedBytesEqual(t testing.TB, actual, expected []byte, msgAndArgs ...interface{}) {
	t.Helper()
	if !bytes.Equal(actual, expected) {
		msg := formatMessage(fmt.Sprintf("expected bytes %v, got %v", expected, actual), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertBytesEqual is an alias for ExpectedBytesEqual.
func AssertBytesEqual(t testing.TB, actual, expected []byte, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedBytesEqual(t, actual, expected, msgAndArgs...)
}

// ExpectedNoError asserts that err is nil.
func ExpectedNoError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		msg := formatMessage(fmt.Sprintf("expected no error, got: %v", err), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertNoError is an alias for ExpectedNoError.
func AssertNoError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedNoError(t, err, msgAndArgs...)
}

// ExpectedError asserts that err is not nil.
func ExpectedError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		msg := formatMessage("expected error, got nil", msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertError is an alias for ExpectedError.
func AssertError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedError(t, err, msgAndArgs...)
}

// ExpectedErrorContains asserts that err is non-nil and contains the specified substring.
func ExpectedErrorContains(t testing.TB, err error, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		msg := formatMessage(fmt.Sprintf("expected error containing %q, got nil", substr), msgAndArgs...)
		t.Fatal(msg)
		return
	}
	if !strings.Contains(err.Error(), substr) {
		msg := formatMessage(fmt.Sprintf("expected error %q to contain %q", err.Error(), substr), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertErrorContains is an alias for ExpectedErrorContains.
func AssertErrorContains(t testing.TB, err error, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedErrorContains(t, err, substr, msgAndArgs...)
}

// ExpectedContains asserts that str contains substr.
func ExpectedContains(t testing.TB, str, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	if !strings.Contains(str, substr) {
		msg := formatMessage(fmt.Sprintf("expected string %q to contain %q", str, substr), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertContains is an alias for ExpectedContains.
func AssertContains(t testing.TB, str, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedContains(t, str, substr, msgAndArgs...)
}

// ExpectedNotContains asserts that str does not contain substr.
func ExpectedNotContains(t testing.TB, str, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	if strings.Contains(str, substr) {
		msg := formatMessage(fmt.Sprintf("expected string %q NOT to contain %q", str, substr), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertNotContains is an alias for ExpectedNotContains.
func AssertNotContains(t testing.TB, str, substr string, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedNotContains(t, str, substr, msgAndArgs...)
}

// ExpectedNil asserts that val is nil (either untyped nil or typed nil pointer/interface).
func ExpectedNil(t testing.TB, val interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if val == nil {
		return
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if v.IsNil() {
			return
		}
	}
	msg := formatMessage(fmt.Sprintf("expected nil, got %v", val), msgAndArgs...)
	t.Fatal(msg)
	return
}

// AssertNil is an alias for ExpectedNil.
func AssertNil(t testing.TB, val interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedNil(t, val, msgAndArgs...)
}

// ExpectedNotNil asserts that val is not nil.
func ExpectedNotNil(t testing.TB, val interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if val == nil {
		msg := formatMessage("expected non-nil value, got nil", msgAndArgs...)
		t.Fatal(msg)
		return
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if v.IsNil() {
			msg := formatMessage("expected non-nil value, got typed nil", msgAndArgs...)
			t.Fatal(msg)
			return
		}
	}
}

// AssertNotNil is an alias for ExpectedNotNil.
func AssertNotNil(t testing.TB, val interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedNotNil(t, val, msgAndArgs...)
}

// ExpectedLen asserts that slice has the expected length.
func ExpectedLen[T any](t testing.TB, slice []T, expectedLen int, msgAndArgs ...interface{}) {
	t.Helper()
	if len(slice) != expectedLen {
		msg := formatMessage(fmt.Sprintf("expected slice length %d, got %d", expectedLen, len(slice)), msgAndArgs...)
		t.Fatal(msg)
		return
	}
}

// AssertLen is an alias for ExpectedLen.
func AssertLen[T any](t testing.TB, slice []T, expectedLen int, msgAndArgs ...interface{}) {
	t.Helper()
	ExpectedLen(t, slice, expectedLen, msgAndArgs...)
}

// StartMockTCPServer starts a local TCP listener on 127.0.0.1:<port> (or 0 for ephemeral)
// and registers automatic cleanup with t.Cleanup.
func StartMockTCPServer(t testing.TB, port int, onConnect ...func(conn net.Conn)) (net.Listener, int, error) {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, 0, err
	}

	actualPort := ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() {
		_ = ln.Close()
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if len(onConnect) > 0 && onConnect[0] != nil {
				go onConnect[0](conn)
			} else {
				_ = conn.Close()
			}
		}
	}()

	return ln, actualPort, nil
}

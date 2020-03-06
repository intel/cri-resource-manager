package testutils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/go-multierror"
)

// VerifyDeepEqual checks that two values (including structures) are equal, or else it fails the test.
func VerifyDeepEqual(t *testing.T, valueName string, expectedValue interface{}, seenValue interface{}) bool {
	if reflect.DeepEqual(expectedValue, seenValue) {
		return true
	}
	t.Errorf("expected %s value %+v, got %+v", valueName, expectedValue, seenValue)
	return false
}

// VerifyError checks a (multi)error has expected properties, or else it fails the test.
func VerifyError(t *testing.T, err error, expectedCount int, expectedSubstrings []string) bool {
	if expectedCount > 0 {
		if err == nil {
			t.Errorf("error expected, got nil")
			return false
		}
		merr, ok := err.(*multierror.Error)
		if !ok {
			t.Errorf("expected %d errors, but got %#v instead of multierror", expectedCount, err)
			return false
		}
		if len(merr.Errors) != expectedCount {
			t.Errorf("expected %d errors, but got %d: %v", expectedCount, len(merr.Errors), merr)
			return false
		}

	} else if expectedCount == 0 {
		if err != nil {
			t.Errorf("expected 0 errors, but got %v", err)
			return false
		}
	}
	for _, substring := range expectedSubstrings {
		if !strings.Contains(err.Error(), substring) {
			t.Errorf("expected error with substring %#v, got \"%v\"", substring, err)
		}
	}
	return true
}

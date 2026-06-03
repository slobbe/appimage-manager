package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestErrorKindOfFindsWrappedServiceError(t *testing.T) {
	base := errors.New("bad input")
	err := NewError(ErrorInvalidInput, "resolve target", base)
	wrapped := errors.Join(errors.New("outer"), err)

	kind, ok := ErrorKindOf(wrapped)
	if !ok || kind != ErrorInvalidInput {
		t.Fatalf("ErrorKindOf = %q, %v; want %q, true", kind, ok, ErrorInvalidInput)
	}
	if !IsErrorKind(wrapped, ErrorInvalidInput) {
		t.Fatal("IsErrorKind returned false")
	}
	if !errors.Is(err, base) {
		t.Fatal("service error does not unwrap base error")
	}
	if !strings.Contains(err.Error(), "resolve target: bad input") {
		t.Fatalf("unexpected error text: %q", err.Error())
	}
}

func TestNewErrorDefaultsToInternalKind(t *testing.T) {
	err := NewError("", "", errors.New("boom"))
	kind, ok := ErrorKindOf(err)
	if !ok || kind != ErrorInternal {
		t.Fatalf("ErrorKindOf = %q, %v; want %q, true", kind, ok, ErrorInternal)
	}
}

func TestSemanticErrorsFromServices(t *testing.T) {
	_, err := (AddWorkflowService{}).resolveIntegrateTarget(context.Background(), "")
	if !IsErrorKind(err, ErrorInvalidInput) {
		t.Fatalf("ResolveIntegrateTarget error kind = %v, want invalid input", err)
	}

	_, err = (StoreListService{}).List(context.Background(), ListRequest{})
	if !IsErrorKind(err, ErrorInternal) {
		t.Fatalf("List error kind = %v, want internal", err)
	}

	_, err = RequireInstallablePackage(nil)
	if !IsErrorKind(err, ErrorInvalidInput) {
		t.Fatalf("RequireInstallablePackage error kind = %v, want invalid input", err)
	}
}

package diagnose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestCommandPrintsHumanDiagnosis(t *testing.T) {
	result := sampleResult()
	service := &fakeService{result: result}
	var stdout, stderr bytes.Buffer
	rt := clienv.New(&stdout, &stderr)
	initialConfig := rt.Config
	cmd := NewCommand(rt, service)
	if cmd.HasLocalFlags() {
		t.Fatal("diagnose should not require local flags")
	}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(forbiddenInput{t: t})
	cmd.SetArgs(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if service.calls != 1 || service.ctx != ctx || service.req != (app.DiagnoseRequest{}) {
		t.Fatalf("service call = %#v, want one call with command context and empty request", service)
	}
	for _, want := range []string{
		"System: Example Linux 1", "Architecture: x86_64", "PASS: FUSE is available",
		"Evidence: libfuse.so.2 found", "Guidance: No changes needed",
		"Command: example-check", "Documentation: https://example.com/appimages",
		"Host checks do not guarantee that an AppImage will launch.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if !reflect.DeepEqual(service.result, sampleResult()) || rt.Config != initialConfig {
		t.Fatal("command mutated result or runtime configuration")
	}
}

func TestCommandSystemFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		system app.DiagnosticSystem
		want   string
	}{
		{name: "id", system: app.DiagnosticSystem{ID: "example"}, want: "System: example"},
		{name: "unknown", system: app.DiagnosticSystem{Error: "os-release unavailable"}, want: "System: unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			service := &fakeService{result: app.DiagnoseResult{System: tc.system}}
			cmd := NewCommand(clienv.New(&stdout, io.Discard), service)
			cmd.SetOut(&stdout)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(nil)
			// An unavailable system may produce an incomplete diagnosis.
			_ = cmd.ExecuteContext(context.Background())
			if !strings.Contains(stdout.String(), tc.want) || !strings.Contains(stdout.String(), "Architecture: unknown") {
				t.Fatalf("stdout = %q, want %q and unknown architecture", stdout.String(), tc.want)
			}
			if tc.system.Error != "" && !strings.Contains(stdout.String(), "System error: "+tc.system.Error) {
				t.Fatalf("stdout = %q, want system error", stdout.String())
			}
		})
	}
}

func TestCommandStatusesAndJSON(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		for _, tc := range []struct {
			status  app.DiagnosticStatus
			code    int
			message string
		}{
			{status: "pass", code: 0},
			{status: "warning", code: 1, message: "diagnosis found compatibility warnings"},
			{status: "unknown", code: 2, message: "diagnosis incomplete"},
		} {
			name := string(tc.status) + "/human"
			if asJSON {
				name = string(tc.status) + "/json"
			}
			t.Run(name, func(t *testing.T) {
				result := sampleResult()
				result.Findings[0].Status = tc.status
				service := &fakeService{result: result}
				var stdout, stderr bytes.Buffer
				rt := clienv.New(&stdout, &stderr)
				rt.Config.JSON = asJSON
				cmd := NewCommand(rt, service)
				cmd.SetOut(&stdout)
				cmd.SetErr(&stderr)
				cmd.SetIn(forbiddenInput{t: t})
				cmd.SetArgs(nil)
				err := cmd.ExecuteContext(context.Background())
				if stderr.Len() != 0 {
					t.Errorf("stderr = %q, want no automatic Cobra errors or usage", stderr.String())
				}
				if strings.Contains(stdout.String(), "Usage:") {
					t.Errorf("stdout = %q, want no automatic Cobra usage", stdout.String())
				}
				if tc.code == 0 {
					if err != nil {
						t.Fatalf("ExecuteContext() error = %v", err)
					}
				} else {
					assertExitError(t, err, tc.code, nil)
					if err.Error() != tc.message {
						t.Errorf("error = %q, want %q", err.Error(), tc.message)
					}
				}
				if !asJSON {
					if !strings.Contains(stdout.String(), strings.ToUpper(string(tc.status))+": ") {
						t.Fatalf("stdout = %q, want uppercase status", stdout.String())
					}
					return
				}
				var gotValue, wantValue any
				if err := json.Unmarshal(stdout.Bytes(), &gotValue); err != nil {
					t.Fatalf("invalid JSON: %v; stdout = %q", err, stdout.String())
				}
				want := `{"system":{"id":"example","version_id":"1","variant_id":"desktop","pretty_name":"Example Linux 1","architecture":"x86_64"},"findings":[{"id":"fuse","status":"` + string(tc.status) + `","summary":"FUSE is available","evidence":"libfuse.so.2 found","command":"example-check","guidance":"No changes needed","documentation_url":"https://example.com/appimages"}]}`
				if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(gotValue, wantValue) {
					t.Fatalf("JSON = %s, want %s", stdout.String(), want)
				}
			})
		}
	}
}

func TestCommandServiceError(t *testing.T) {
	wantErr := errors.New("host inspection failed")
	service := &fakeService{err: wantErr}
	var stdout bytes.Buffer
	cmd := NewCommand(clienv.New(&stdout, io.Discard), service)
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(nil)
	err := cmd.ExecuteContext(context.Background())
	assertExitError(t, err, 2, wantErr)
	if err.Error() != wantErr.Error() || stdout.Len() != 0 {
		t.Fatalf("error = %v, stdout = %q; want original error and no output", err, stdout.String())
	}
}

func TestCommandOutputError(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		wantErr := errors.New("output failed")
		result := sampleResult()
		result.Findings[0].Status = "warning"
		rt := clienv.New(failingWriter{err: wantErr}, io.Discard)
		rt.Config.JSON = asJSON
		cmd := NewCommand(rt, &fakeService{result: result})
		cmd.SetOut(rt.Out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(nil)
		err := cmd.ExecuteContext(context.Background())
		assertExitError(t, err, 2, wantErr)
		if err.Error() != wantErr.Error() {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	}
}

func TestCommandRejectsArguments(t *testing.T) {
	service := &fakeService{}
	cmd := NewCommand(clienv.New(io.Discard, io.Discard), service)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetIn(forbiddenInput{t: t})
	cmd.SetArgs([]string{"unexpected"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("ExecuteContext() succeeded with an unexpected argument")
	}
	if service.calls != 0 {
		t.Fatal("service called for invalid arguments")
	}
}

func assertExitError(t *testing.T, err error, code int, cause error) {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != code {
		t.Fatalf("error = %v, want *ExitError with code %d", err, code)
	}
	if exitErr.Unwrap() != cause || (cause != nil && !errors.Is(err, cause)) {
		t.Fatalf("error = %v, want wrapped cause %v", err, cause)
	}
}

func sampleResult() app.DiagnoseResult {
	return app.DiagnoseResult{
		System: app.DiagnosticSystem{
			ID: "example", VersionID: "1", VariantID: "desktop",
			PrettyName: "Example Linux 1", Architecture: "x86_64",
		},
		Findings: []app.DiagnosticFinding{{
			ID: "fuse", Status: "pass", Summary: "FUSE is available",
			Evidence: "libfuse.so.2 found", Command: "example-check",
			Guidance: "No changes needed", DocumentationURL: "https://example.com/appimages",
		}},
	}
}

type fakeService struct {
	result app.DiagnoseResult
	err    error
	calls  int
	ctx    context.Context
	req    app.DiagnoseRequest
}

var _ service = (*fakeService)(nil)

func (s *fakeService) Diagnose(ctx context.Context, req app.DiagnoseRequest) (app.DiagnoseResult, error) {
	s.calls++
	s.ctx = ctx
	s.req = req
	return s.result, s.err
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type forbiddenInput struct{ t *testing.T }

func (r forbiddenInput) Read([]byte) (int, error) {
	r.t.Fatal("diagnose must not read prompt input")
	return 0, io.EOF
}

package paths

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
)

func TestCommandCallsServiceAndPrintsTextPaths(t *testing.T) {
	service := &fakeService{pathsResult: samplePathsResult()}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if !service.pathsCalled {
		t.Fatal("service.Paths was not called")
	}
	output := stdout.String()
	for _, want := range []string{
		"Config file:  /home/user/.config/aim/config.toml",
		"AppImage dir: /home/user/Applications",
		"Cache dir:    /home/user/.cache/aim",
		"Desktop dir:  /home/user/.local/share/applications",
		"Icon dir:     /home/user/.local/share/icons/hicolor/256x256/apps",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
}

func TestCommandPrintsJSONPaths(t *testing.T) {
	service := &fakeService{pathsResult: samplePathsResult()}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		ConfigFile  string `json:"config_file"`
		AppImageDir string `json:"appimage_dir"`
		CacheDir    string `json:"cache_dir"`
		DesktopDir  string `json:"desktop_dir"`
		IconDir     string `json:"icon_dir"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	want := samplePathsResult()
	if payload.ConfigFile != want.ConfigFile || payload.AppImageDir != want.AppImageDir || payload.CacheDir != want.CacheDir || payload.DesktopDir != want.DesktopDir || payload.IconDir != want.IconDir {
		t.Fatalf("payload = %#v, want paths result %#v", payload, want)
	}
}

func TestCommandReturnsServiceError(t *testing.T) {
	wantErr := errors.New("paths failed")
	service := &fakeService{pathsErr: wantErr}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

func samplePathsResult() app.PathsResult {
	return app.PathsResult{
		ConfigFile:  "/home/user/.config/aim/config.toml",
		AppImageDir: "/home/user/Applications",
		CacheDir:    "/home/user/.cache/aim",
		DesktopDir:  "/home/user/.local/share/applications",
		IconDir:     "/home/user/.local/share/icons/hicolor/256x256/apps",
	}
}

type fakeService struct {
	pathsCalled bool
	pathsResult app.PathsResult
	pathsErr    error
}

var _ app.PathProvider = (*fakeService)(nil)

func (s *fakeService) Paths(ctx context.Context, req app.PathsRequest) (app.PathsResult, error) {
	s.pathsCalled = true
	if s.pathsErr != nil {
		return app.PathsResult{}, s.pathsErr
	}
	return s.pathsResult, nil
}

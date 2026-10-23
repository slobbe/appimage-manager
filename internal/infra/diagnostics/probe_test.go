package diagnostics

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"golang.org/x/sys/unix"
)

func TestResolveLdconfig(t *testing.T) {
	for _, tt := range []struct {
		name      string
		found     string
		wantCalls string
	}{
		{"path", "ldconfig", "ldconfig"},
		{"sbin fallback", "/sbin/ldconfig", "ldconfig,/sbin/ldconfig"},
		{"usr sbin fallback", "/usr/sbin/ldconfig", "ldconfig,/sbin/ldconfig,/usr/sbin/ldconfig"},
		{"unavailable", "", "ldconfig,/sbin/ldconfig,/usr/sbin/ldconfig"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			path, err := resolveLdconfig(func(candidate string) (string, error) {
				calls = append(calls, candidate)
				if candidate == tt.found {
					return "/resolved/ldconfig", nil
				}
				if candidate == "/sbin/ldconfig" {
					return "", os.ErrPermission
				}
				return "", exec.ErrNotFound
			})
			if got := strings.Join(calls, ","); got != tt.wantCalls {
				t.Fatalf("lookups = %s, want %s", got, tt.wantCalls)
			}
			if tt.found != "" {
				if err != nil || path != "/resolved/ldconfig" {
					t.Fatalf("got %q, %v", path, err)
				}
				return
			}
			if path != "" || !errors.Is(err, exec.ErrNotFound) || !errors.Is(err, os.ErrPermission) {
				t.Fatalf("resolution errors not preserved: %q, %v", path, err)
			}
			for _, candidate := range calls {
				if !strings.Contains(err.Error(), candidate+":") {
					t.Fatalf("missing candidate %q in %v", candidate, err)
				}
			}
		})
	}
}

func TestLibraryTimeout(t *testing.T) {
	d := fixtures()
	d.cache = func(ctx context.Context) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("cache query does not have a five-second time limit")
		}
		<-ctx.Done()
		// CommandContext can report the killed process instead of the context error.
		return []byte("partial cache output"), errors.New("signal: killed")
	}
	report, err := inspect(context.Background(), d)
	if err != nil {
		t.Fatalf("internal timeout propagated as caller cancellation: %v", err)
	}
	if report.Library.State != app.DiagnosticUnknown {
		t.Fatalf("library = %+v", report.Library)
	}
	for _, evidence := range []string{"context deadline exceeded", "signal: killed", "partial cache output"} {
		if !strings.Contains(report.Library.Evidence, evidence) {
			t.Fatalf("missing %q in %+v", evidence, report.Library)
		}
	}
	if report.Device.State != app.DiagnosticPass {
		t.Fatalf("device probe did not continue after cache timeout: %+v", report.Device)
	}
}

func TestInspectCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := inspect(ctx, fixtures()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller deadline = %v", err)
	}
}

func fixtures() dependencies {
	return dependencies{
		readFile: func(string) ([]byte, error) { return []byte("ID=fixture\n"), nil },
		cache:    func(context.Context) ([]byte, error) { return []byte("0 libs found in cache\n"), nil },
		elf:      func(string) (elfIdentity, error) { return elfIdentity{}, errors.New("unexpected ELF read") },
		stat: func(_ string, stat *unix.Stat_t) error {
			stat.Mode = unix.S_IFCHR | 0666
			stat.Rdev = unix.Mkdev(10, 229)
			return nil
		},
		access: func(string, uint32) error { return nil },
		arch:   "amd64",
	}
}

func TestSystem(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
		want app.DiagnosticSystem
		bad  bool
	}{
		{"unquoted", "ID=fedora\nVERSION_ID=43\nVARIANT_ID=workstation", app.DiagnosticSystem{ID: "fedora", VersionID: "43", VariantID: "workstation"}, false},
		{"quoted", "# comment\nID='debian'\nVERSION_ID=\"13\"\nPRETTY_NAME=\"Debian \\\"Test\\\" \\$HOME\"\n", app.DiagnosticSystem{ID: "debian", VersionID: "13", PrettyName: "Debian \"Test\" $HOME"}, false},
		{"literal shell text", "PRETTY_NAME='$(touch /tmp/not-executed); `id`'", app.DiagnosticSystem{PrettyName: "$(touch /tmp/not-executed); `id`"}, false},
		{"escaped space", "PRETTY_NAME=Example\\ OS", app.DiagnosticSystem{PrettyName: "Example OS"}, false},
		{"quoted backslash", "PRETTY_NAME=\"Example\\OS\"", app.DiagnosticSystem{PrettyName: "Example\\OS"}, false},
		{"trailing comment", "ID=example # comment", app.DiagnosticSystem{ID: "example"}, false},
		{"unquoted command", "ID=$(id)", app.DiagnosticSystem{}, true},
		{"empty", "\n# comment\n", app.DiagnosticSystem{}, false},
		{"unterminated", "ID=good\nPRETTY_NAME=\"broken", app.DiagnosticSystem{}, true},
		{"not assignment", "ID=good\ninvalid", app.DiagnosticSystem{}, true},
		{"invalid key", "1ID=bad", app.DiagnosticSystem{}, true},
		{"whitespace", "PRETTY_NAME=not quoted", app.DiagnosticSystem{}, true},
		{"trailing escape", "ID=bad\\", app.DiagnosticSystem{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := fixtures()
			d.readFile = func(path string) ([]byte, error) {
				if path != "/etc/os-release" {
					t.Fatalf("unexpected path %s", path)
				}
				return []byte(tt.data), nil
			}
			got := inspectSystem(d)
			if (got.Error != "") != tt.bad {
				t.Fatalf("error = %q, want error %v", got.Error, tt.bad)
			}
			if tt.bad && !strings.Contains(got.Error, "parse /etc/os-release: line") {
				t.Fatalf("missing context: %s", got.Error)
			}
			got.Error = ""
			tt.want.Architecture = "amd64"
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSystemFallback(t *testing.T) {
	for _, tt := range []struct {
		name      string
		first     error
		second    error
		calls     int
		wantError string
	}{
		{"absent", os.ErrNotExist, nil, 2, ""},
		{"permission denied", os.ErrPermission, nil, 1, "permission denied"},
		{"unexpected", unix.EIO, nil, 1, "input/output error"},
		{"both missing", os.ErrNotExist, os.ErrNotExist, 2, "/usr/lib/os-release"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := fixtures()
			calls := 0
			d.readFile = func(path string) ([]byte, error) {
				calls++
				if calls == 1 {
					if path != "/etc/os-release" {
						t.Fatal(path)
					}
					return nil, &os.PathError{Op: "read", Path: path, Err: tt.first}
				}
				if path != "/usr/lib/os-release" {
					t.Fatal(path)
				}
				return []byte("ID=fallback"), tt.second
			}
			got := inspectSystem(d)
			if calls != tt.calls {
				t.Fatalf("calls = %d, want %d", calls, tt.calls)
			}
			if tt.wantError == "" {
				if got.Error != "" || got.ID != "fallback" {
					t.Fatalf("got %+v", got)
				}
			} else if !strings.Contains(got.Error, tt.wantError) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestLibrary(t *testing.T) {
	native, _ := nativeELF("amd64")
	foreign, _ := nativeELF("arm64")
	wrongClass := native
	wrongClass.class = elf.ELFCLASS32
	wrongEndian := native
	wrongEndian.data = elf.ELFDATA2MSB
	for _, tt := range []struct {
		name     string
		cache    string
		cacheErr error
		identity elfIdentity
		elfErr   error
		want     app.DiagnosticStatus
		evidence string
	}{
		{"native", "libfuse.so.2 (libc6,x86-64) => /native.so", nil, native, nil, app.DiagnosticPass, "loader success not tested"},
		{"missing", "12 libs found in cache\nlibfuse3.so.3 => /fuse3.so", nil, native, nil, app.DiagnosticWarning, "not detected in ldconfig cache"},
		{"foreign", "libfuse.so.2 (libc6) => /foreign.so", nil, foreign, nil, app.DiagnosticWarning, "not detected in ldconfig cache"},
		{"wrong class", "libfuse.so.2 => /wrong.so", nil, wrongClass, nil, app.DiagnosticWarning, "not detected in ldconfig cache"},
		{"wrong endian", "libfuse.so.2 => /wrong.so", nil, wrongEndian, nil, app.DiagnosticWarning, "not detected in ldconfig cache"},
		{"inaccessible", "libfuse.so.2 => /denied.so", nil, native, os.ErrPermission, app.DiagnosticUnknown, "permission denied"},
		{"stale cache", "libfuse.so.2 => /gone.so", nil, native, os.ErrNotExist, app.DiagnosticUnknown, "does not exist"},
		{"invalid ELF", "libfuse.so.2 => /invalid.so", nil, native, errors.New("bad magic"), app.DiagnosticUnknown, "bad magic"},
		{"unavailable", "", errors.New("executable file not found"), native, nil, app.DiagnosticUnknown, "executable file not found"},
		{"query failed", "cache corrupt", errors.New("exit status 1"), native, nil, app.DiagnosticUnknown, "cache corrupt"},
		{"malformed entry", "libfuse.so.2 (libc6)", nil, native, nil, app.DiagnosticUnknown, "unrecognized"},
		{"relative target", "libfuse.so.2 => relative.so", nil, native, nil, app.DiagnosticUnknown, "unrecognized"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := fixtures()
			d.cache = func(context.Context) ([]byte, error) { return []byte(tt.cache), tt.cacheErr }
			d.elf = func(string) (elfIdentity, error) { return tt.identity, tt.elfErr }
			got := inspectLibrary(context.Background(), d)
			if got.State != tt.want || !strings.Contains(got.Evidence, tt.evidence) {
				t.Fatalf("got %+v, want %s containing %q", got, tt.want, tt.evidence)
			}
		})
	}
}

func TestLibraryMixedCandidates(t *testing.T) {
	d := fixtures()
	d.cache = func(context.Context) ([]byte, error) {
		return []byte("libfuse.so.2 => /unreadable.so\nlibfuse.so.2 => /native.so"), nil
	}
	d.elf = func(path string) (elfIdentity, error) {
		if path == "/unreadable.so" {
			return elfIdentity{}, os.ErrPermission
		}
		identity, _ := nativeELF(d.arch)
		return identity, nil
	}
	if got := inspectLibrary(context.Background(), d); got.State != app.DiagnosticPass {
		t.Fatalf("got %+v", got)
	}
	d.arch = "unsupported"
	if got := inspectLibrary(context.Background(), d); got.State != app.DiagnosticUnknown {
		t.Fatalf("got %+v", got)
	}
}

func TestReadELF(t *testing.T) {
	for _, tt := range []struct {
		arch    string
		class   elf.Class
		machine elf.Machine
		data    elf.Data
	}{
		{"amd64", elf.ELFCLASS64, elf.EM_X86_64, elf.ELFDATA2LSB},
		{"386", elf.ELFCLASS32, elf.EM_386, elf.ELFDATA2LSB},
		{"arm64", elf.ELFCLASS64, elf.EM_AARCH64, elf.ELFDATA2LSB},
		{"s390x", elf.ELFCLASS64, elf.EM_S390, elf.ELFDATA2MSB},
	} {
		t.Run(tt.arch, func(t *testing.T) {
			data := make([]byte, 64)
			copy(data, "\x7fELF")
			data[4], data[5], data[6] = byte(tt.class), byte(tt.data), 1
			var order binary.ByteOrder = binary.LittleEndian
			if tt.data == elf.ELFDATA2MSB {
				order = binary.BigEndian
			}
			order.PutUint16(data[16:], uint16(elf.ET_DYN))
			order.PutUint16(data[18:], uint16(tt.machine))
			order.PutUint32(data[20:], 1)
			path := filepath.Join(t.TempDir(), "libfuse.so.2")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readELF(path)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := nativeELF(tt.arch)
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
			if err := os.WriteFile(path, []byte("not ELF"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readELF(path); err == nil {
				t.Fatal("accepted non-ELF file")
			}
		})
	}
	if _, err := readELF(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted missing target")
	}
}

func TestDevice(t *testing.T) {
	for _, tt := range []struct {
		name      string
		statErr   error
		accessErr error
		mode      uint32
		rdev      uint64
		want      app.DiagnosticStatus
		evidence  string
	}{
		{"accessible", nil, nil, unix.S_IFCHR, unix.Mkdev(10, 229), app.DiagnosticPass, "opening and mounting not tested"},
		{"missing", unix.ENOENT, nil, 0, 0, app.DiagnosticWarning, "no such file"},
		{"stat denied", unix.EACCES, nil, 0, 0, app.DiagnosticWarning, "permission denied"},
		{"stat failed", unix.EIO, nil, 0, 0, app.DiagnosticUnknown, "input/output error"},
		{"regular file", nil, nil, unix.S_IFREG, 0, app.DiagnosticWarning, "not the FUSE character device"},
		{"block device", nil, nil, unix.S_IFBLK, unix.Mkdev(10, 229), app.DiagnosticWarning, "not the FUSE character device"},
		{"wrong character device", nil, nil, unix.S_IFCHR, unix.Mkdev(1, 3), app.DiagnosticWarning, "not the FUSE character device"},
		{"access denied", nil, unix.EACCES, unix.S_IFCHR, unix.Mkdev(10, 229), app.DiagnosticWarning, "permission denied"},
		{"access forbidden", nil, unix.EPERM, unix.S_IFCHR, unix.Mkdev(10, 229), app.DiagnosticWarning, "operation not permitted"},
		{"removed", nil, unix.ENOENT, unix.S_IFCHR, unix.Mkdev(10, 229), app.DiagnosticWarning, "no such file"},
		{"access failed", nil, unix.EIO, unix.S_IFCHR, unix.Mkdev(10, 229), app.DiagnosticUnknown, "input/output error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := fixtures()
			accessed := false
			d.stat = func(path string, stat *unix.Stat_t) error {
				if path != "/dev/fuse" {
					t.Fatal(path)
				}
				stat.Mode, stat.Rdev = tt.mode, tt.rdev
				return tt.statErr
			}
			d.access = func(path string, mode uint32) error {
				accessed = true
				if path != "/dev/fuse" || mode != unix.R_OK|unix.W_OK {
					t.Fatalf("access(%q, %d)", path, mode)
				}
				return tt.accessErr
			}
			got := inspectDevice(d)
			if got.State != tt.want || !strings.Contains(got.Evidence, tt.evidence) {
				t.Fatalf("got %+v", got)
			}
			if accessed && (tt.statErr != nil || tt.mode != unix.S_IFCHR || tt.rdev != unix.Mkdev(10, 229)) {
				t.Fatal("access checked before device validation")
			}
		})
	}
}

func TestInspectCancellation(t *testing.T) {
	for _, stage := range []string{"before", "system", "library", "device"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := fixtures()
			calls := 0
			d.readFile = func(string) ([]byte, error) {
				calls++
				if stage == "system" {
					cancel()
				}
				return []byte("ID=fixture"), nil
			}
			d.cache = func(ctx context.Context) ([]byte, error) {
				calls++
				if stage != "library" {
					t.Fatal("unexpected cache query")
				}
				cancel()
				return nil, ctx.Err()
			}
			if stage == "device" {
				d.cache = func(context.Context) ([]byte, error) { calls++; return nil, nil }
				d.stat = func(string, *unix.Stat_t) error { calls++; cancel(); return unix.EIO }
			}
			if stage == "before" {
				cancel()
			}
			_, err := inspect(ctx, d)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
			wantCalls := map[string]int{"before": 0, "system": 1, "library": 2, "device": 3}[stage]
			if calls != wantCalls {
				t.Fatalf("calls = %d, want %d", calls, wantCalls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Probe{}).Inspect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Probe cancellation = %v", err)
	}
}

func TestInspectReport(t *testing.T) {
	report, err := inspect(context.Background(), fixtures())
	if err != nil {
		t.Fatal(err)
	}
	if report.System.ID != "fixture" || report.Library.State != app.DiagnosticWarning || report.Device.State != app.DiagnosticPass {
		t.Fatalf("got %+v", report)
	}
}

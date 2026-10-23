// Package diagnostics provides read-only evidence about host FUSE support.
package diagnostics

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"golang.org/x/sys/unix"
)

// Probe inspects the host without opening /dev/fuse or changing system state.
type Probe struct{}

var _ app.HostDiagnostics = Probe{}

type elfIdentity struct {
	class   elf.Class
	machine elf.Machine
	data    elf.Data
}

type dependencies struct {
	readFile func(string) ([]byte, error)
	cache    func(context.Context) ([]byte, error)
	elf      func(string) (elfIdentity, error)
	stat     func(string, *unix.Stat_t) error
	access   func(string, uint32) error
	arch     string
}

func (Probe) Inspect(ctx context.Context) (app.HostDiagnosticsReport, error) {
	return inspect(ctx, dependencies{
		readFile: os.ReadFile,
		cache: func(ctx context.Context) ([]byte, error) {
			path, err := resolveLdconfig(exec.LookPath)
			if err != nil {
				return nil, err
			}
			return exec.CommandContext(ctx, path, "-p").CombinedOutput()
		},
		elf:    readELF,
		stat:   unix.Stat,
		access: unix.Access,
		arch:   runtime.GOARCH,
	})
}

func resolveLdconfig(lookPath func(string) (string, error)) (string, error) {
	var failures []error
	for _, candidate := range []string{"ldconfig", "/sbin/ldconfig", "/usr/sbin/ldconfig"} {
		path, err := lookPath(candidate)
		if err == nil {
			return path, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", candidate, err))
	}
	return "", fmt.Errorf("resolve ldconfig: %w", errors.Join(failures...))
}

func inspect(ctx context.Context, d dependencies) (app.HostDiagnosticsReport, error) {
	var report app.HostDiagnosticsReport
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.System = inspectSystem(d)
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Library = inspectLibrary(ctx, d)
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Device = inspectDevice(d)
	return report, ctx.Err()
}

func inspectSystem(d dependencies) app.DiagnosticSystem {
	system := app.DiagnosticSystem{Architecture: d.arch}
	path := "/etc/os-release"
	data, err := d.readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		path = "/usr/lib/os-release"
		data, err = d.readFile(path)
	}
	if err != nil {
		system.Error = fmt.Sprintf("read %s: %v", path, err)
		return system
	}
	values, err := parseOSRelease(string(data))
	if err != nil {
		system.Error = fmt.Sprintf("parse %s: %v", path, err)
		return system
	}
	system.ID = values["ID"]
	system.VersionID = values["VERSION_ID"]
	system.VariantID = values["VARIANT_ID"]
	system.PrettyName = values["PRETTY_NAME"]
	return system
}

// Parse shell-style quoting as data; expansion and execution are never performed.
func parseOSRelease(data string) (map[string]string, error) {
	values := make(map[string]string)
	for n, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("line %d: expected key=value", n+1)
		}
		for i, c := range key {
			if c != '_' && !(c >= 'A' && c <= 'Z') && !(c >= 'a' && c <= 'z') && !(i > 0 && c >= '0' && c <= '9') {
				return nil, fmt.Errorf("line %d: invalid key", n+1)
			}
		}
		value, err := parseValue(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		values[key] = value
	}
	return values, nil
}

func parseValue(raw string) (string, error) {
	var value strings.Builder
	var quote byte
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\\' && quote != '\'' {
			if i+1 == len(raw) {
				return "", errors.New("trailing escape")
			}
			i++
			if quote == '"' && !strings.ContainsRune("$`\"\\", rune(raw[i])) {
				value.WriteByte('\\')
			}
			value.WriteByte(raw[i])
		} else if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				value.WriteByte(c)
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == ' ' || c == '\t' {
			rest := strings.TrimSpace(raw[i:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", errors.New("unquoted whitespace")
			}
			break
		} else if strings.ContainsRune("$`;|&<>()", rune(c)) {
			return "", errors.New("unquoted shell metacharacter")
		} else {
			value.WriteByte(c)
		}
	}
	if quote != 0 {
		return "", errors.New("unterminated quote")
	}
	return value.String(), nil
}

func readELF(path string) (elfIdentity, error) {
	file, err := elf.Open(path)
	if err != nil {
		return elfIdentity{}, err
	}
	defer file.Close()
	return elfIdentity{file.Class, file.Machine, file.Data}, nil
}

func nativeELF(arch string) (elfIdentity, bool) {
	little := elf.ELFDATA2LSB
	big := elf.ELFDATA2MSB
	identities := map[string]elfIdentity{
		"386":      {elf.ELFCLASS32, elf.EM_386, little},
		"amd64":    {elf.ELFCLASS64, elf.EM_X86_64, little},
		"arm":      {elf.ELFCLASS32, elf.EM_ARM, little},
		"arm64":    {elf.ELFCLASS64, elf.EM_AARCH64, little},
		"loong64":  {elf.ELFCLASS64, elf.EM_LOONGARCH, little},
		"mips":     {elf.ELFCLASS32, elf.EM_MIPS, big},
		"mipsle":   {elf.ELFCLASS32, elf.EM_MIPS, little},
		"mips64":   {elf.ELFCLASS64, elf.EM_MIPS, big},
		"mips64le": {elf.ELFCLASS64, elf.EM_MIPS, little},
		"ppc64":    {elf.ELFCLASS64, elf.EM_PPC64, big},
		"ppc64le":  {elf.ELFCLASS64, elf.EM_PPC64, little},
		"riscv64":  {elf.ELFCLASS64, elf.EM_RISCV, little},
		"s390x":    {elf.ELFCLASS64, elf.EM_S390, big},
	}
	identity, ok := identities[arch]
	return identity, ok
}

func inspectLibrary(ctx context.Context, d dependencies) app.DiagnosticProbe {
	cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := d.cache(cacheCtx)
	if cacheCtx.Err() != nil {
		err = errors.Join(err, cacheCtx.Err())
	}
	if err != nil {
		return app.DiagnosticProbe{State: app.DiagnosticUnknown, Evidence: fmt.Sprintf("ldconfig -p: %v; %s", err, strings.TrimSpace(string(output)))}
	}
	native, ok := nativeELF(d.arch)
	if !ok {
		return app.DiagnosticProbe{State: app.DiagnosticUnknown, Evidence: "cannot validate native ELF architecture: " + d.arch}
	}
	var ambiguous []string
	for _, line := range strings.Split(string(output), "\n") {
		if ctx.Err() != nil {
			return app.DiagnosticProbe{State: app.DiagnosticUnknown, Evidence: ctx.Err().Error()}
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "libfuse.so.2" {
			continue
		}
		_, path, ok := strings.Cut(line, "=>")
		path = strings.TrimSpace(path)
		if !ok || !filepath.IsAbs(path) {
			ambiguous = append(ambiguous, "unrecognized libfuse.so.2 cache entry: "+strings.TrimSpace(line))
			continue
		}
		identity, err := d.elf(path)
		if err != nil {
			ambiguous = append(ambiguous, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if identity == native {
			return app.DiagnosticProbe{State: app.DiagnosticPass, Evidence: "ldconfig cache lists native libfuse.so.2 at " + path + "; ELF architecture verified, loader success not tested"}
		}
	}
	if len(ambiguous) > 0 {
		return app.DiagnosticProbe{State: app.DiagnosticUnknown, Evidence: strings.Join(ambiguous, "; ")}
	}
	return app.DiagnosticProbe{State: app.DiagnosticWarning, Evidence: "native libfuse.so.2 not detected in ldconfig cache"}
}

func inspectDevice(d dependencies) app.DiagnosticProbe {
	var stat unix.Stat_t
	if err := d.stat("/dev/fuse", &stat); err != nil {
		state := app.DiagnosticUnknown
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			state = app.DiagnosticWarning
		}
		return app.DiagnosticProbe{State: state, Evidence: fmt.Sprintf("stat /dev/fuse: %v", err)}
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFCHR || unix.Major(uint64(stat.Rdev)) != 10 || unix.Minor(uint64(stat.Rdev)) != 229 {
		return app.DiagnosticProbe{State: app.DiagnosticWarning, Evidence: "/dev/fuse is not the FUSE character device (10:229)"}
	}
	if err := d.access("/dev/fuse", unix.R_OK|unix.W_OK); err != nil {
		state := app.DiagnosticUnknown
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			state = app.DiagnosticWarning
		}
		return app.DiagnosticProbe{State: state, Evidence: fmt.Sprintf("read/write access to /dev/fuse: %v", err)}
	}
	return app.DiagnosticProbe{State: app.DiagnosticPass, Evidence: "/dev/fuse is the FUSE character device with read/write access; opening and mounting not tested"}
}

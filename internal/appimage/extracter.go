package appimage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// MarshalJSON implements json.Marshaler so you can call json.Marshal(f).
func (f *File) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(f.Data, "", "  ")
}

// ToJSON returns the parsed .desktop file as a JSON string.
func (f *File) ToJSON(pretty bool) (string, error) {
	if pretty {
		b, err := json.MarshalIndent(f.Data, "", "  ")
		return string(b), err
	}
	b, err := json.Marshal(f.Data)
	return string(b), err
}

const DesktopEntryGroup = "Desktop Entry"

type File struct {
	// Data[group][key] = value
	Data map[string]map[string]string
}

// Parse reads a .desktop (or .directory) file from r.
func Parse(r io.Reader) (*File, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	f := &File{Data: make(map[string]map[string]string)}
	section := ""
	var cont strings.Builder
	var haveCont bool

	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		// Support CRLF
		line := strings.TrimRightFunc(raw, func(r rune) bool { return r == '\r' || r == '\n' })

		// Handle line continuations: trailing unescaped backslash
		if hasTrailingBackslash(line) {
			cont.WriteString(strings.TrimSuffix(line, `\`))
			haveCont = true
			continue
		}
		if haveCont {
			cont.WriteString(line)
			line = cont.String()
			cont.Reset()
			haveCont = false
		}

		line = trimBOM(line)
		line = strings.TrimLeftFunc(line, unicode.IsSpace)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header [Group]
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				return nil, errors.New("empty section name")
			}
			if _, ok := f.Data[section]; !ok {
				f.Data[section] = make(map[string]string)
			}
			continue
		}
		if section == "" {
			// Per spec, keys outside a group are invalid; ignore.
			continue
		}

		// Key=Value
		k, v, ok := cutOnce(line, '=')
		if !ok {
			// Invalid line; ignore conservatively.
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		f.Data[section][key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return f, nil
}

// DesktopEntry is a convenience wrapper bound to the [Desktop Entry] group.
type DesktopEntry struct {
	f *File
}

// Desktop returns a DesktopEntry helper. Safe even if the group is missing.
func (f *File) Desktop() DesktopEntry { return DesktopEntry{f: f} }

// Groups lists all section names.
func (f *File) Groups() []string {
	out := make([]string, 0, len(f.Data))
	for g := range f.Data {
		out = append(out, g)
	}
	return out
}

// Get returns the raw value for group/key.
func (f *File) Get(group, key string) (string, bool) {
	m, ok := f.Data[group]
	if !ok {
		return "", false
	}
	v, ok := m[key]
	return v, ok
}

// GetLocale implements locale fallback: key[ll_CC] → key[ll] → key.
func (f *File) GetLocale(group, key, locale string) (string, bool) {
	m, ok := f.Data[group]
	if !ok {
		return "", false
	}
	if locale != "" {
		ll, cc := splitLocale(locale)
		if cc != "" {
			if v, ok := m[key+"["+ll+"_"+cc+"]"]; ok {
				return v, true
			}
		}
		if v, ok := m[key+"["+ll+"]"]; ok {
			return v, true
		}
	}
	if v, ok := m[key]; ok {
		return v, true
	}
	return "", false
}

// String returns an unescaped string (per spec escape rules).
func (f *File) String(group, key string) (string, bool) {
	v, ok := f.Get(group, key)
	if !ok {
		return "", false
	}
	return unescape(v), true
}

// StringLocale returns an unescaped localized string with fallback.
func (f *File) StringLocale(group, key, locale string) (string, bool) {
	v, ok := f.GetLocale(group, key, locale)
	if !ok {
		return "", false
	}
	return unescape(v), true
}

// List parses a semicolon-separated list (trailing empty part is ignored).
func (f *File) List(group, key string) ([]string, bool) {
	v, ok := f.Get(group, key)
	if !ok {
		return nil, false
	}
	parts := splitSemicolonList(v)
	for i := range parts {
		parts[i] = unescape(parts[i])
	}
	return parts, true
}

// Bool parses a boolean (true/false, case-insensitive).
func (f *File) Bool(group, key string) (bool, bool) {
	v, ok := f.Get(group, key)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// Int parses a decimal integer.
func (f *File) Int(group, key string) (int, bool) {
	v, ok := f.Get(group, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

// Convenience getters for [Desktop Entry].
func (de DesktopEntry) Name(locale string) (string, bool) {
	return de.f.StringLocale(DesktopEntryGroup, "Name", locale)
}
func (de DesktopEntry) Comment(locale string) (string, bool) {
	return de.f.StringLocale(DesktopEntryGroup, "Comment", locale)
}
func (de DesktopEntry) Exec() (string, bool)    { return de.f.String(DesktopEntryGroup, "Exec") }
func (de DesktopEntry) TryExec() (string, bool) { return de.f.String(DesktopEntryGroup, "TryExec") }
func (de DesktopEntry) Icon() (string, bool)    { return de.f.String(DesktopEntryGroup, "Icon") }
func (de DesktopEntry) Type() (string, bool)    { return de.f.String(DesktopEntryGroup, "Type") }
func (de DesktopEntry) Categories() ([]string, bool) {
	return de.f.List(DesktopEntryGroup, "Categories")
}
func (de DesktopEntry) MimeType() ([]string, bool) { return de.f.List(DesktopEntryGroup, "MimeType") }
func (de DesktopEntry) OnlyShowIn() ([]string, bool) {
	return de.f.List(DesktopEntryGroup, "OnlyShowIn")
}
func (de DesktopEntry) NotShowIn() ([]string, bool) { return de.f.List(DesktopEntryGroup, "NotShowIn") }
func (de DesktopEntry) Terminal() (bool, bool)      { return de.f.Bool(DesktopEntryGroup, "Terminal") }
func (de DesktopEntry) NoDisplay() (bool, bool)     { return de.f.Bool(DesktopEntryGroup, "NoDisplay") }
func (de DesktopEntry) Hidden() (bool, bool)        { return de.f.Bool(DesktopEntryGroup, "Hidden") }
func (de DesktopEntry) StartupNotify() (bool, bool) {
	return de.f.Bool(DesktopEntryGroup, "StartupNotify")
}
func (de DesktopEntry) StartupWMClass() (string, bool) {
	return de.f.String(DesktopEntryGroup, "StartupWMClass")
}
func (de DesktopEntry) Actions() ([]string, bool) { return de.f.List(DesktopEntryGroup, "Actions") }

// Helpers

func cutOnce(s string, sep byte) (before, after string, found bool) {
	i := strings.IndexByte(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func splitLocale(s string) (ll, cc string) {
	s = strings.ReplaceAll(s, "-", "_")
	if i := strings.IndexByte(s, '_'); i >= 0 {
		return s[:i], s[i+1:]
	}
	// language only
	if len(s) >= 2 {
		return s[:2], ""
	}
	return s, ""
}

func hasTrailingBackslash(s string) bool {
	if !strings.HasSuffix(s, `\`) {
		return false
	}
	// Count trailing backslashes; odd means escaped continuation
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// Per spec-style unescaping: \s(space) \t \n \r \\ \; and \:
func unescape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case ';':
			b.WriteByte(';')
		case ':':
			b.WriteByte(':')
		default:
			// Unknown escape: keep literal char
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func splitSemicolonList(v string) []string {
	// Split on unescaped ';'. Trailing empty item is ignored by spec.
	var parts []string
	var cur strings.Builder
	escaped := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		if escaped {
			cur.WriteByte('\\')
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == ';' {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	last := cur.String()
	if last != "" {
		parts = append(parts, last)
	}
	return parts
}

func trimBOM(s string) string {
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		return s[3:]
	}
	return s
}

// SaveDesktop writes the current data back into .desktop syntax.
func (f *File) SaveDesktop(path string) error {
	var sb strings.Builder

	for group, kv := range f.Data {
		sb.WriteString("[" + group + "]\n")
		for k, v := range kv {
			sb.WriteString(k + "=" + v + "\n")
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func ExtractMetadata(appImagePath string) (*File, error) {
	tmpDir, err := os.MkdirTemp("", "appimage_extract_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command(appImagePath, "--appimage-extract")
	cmd.Dir = tmpDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("extract failed: %v: %s", err, stderr.String())
	}

	root := filepath.Join(tmpDir, "squashfs-root")

	var desktopFile string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".desktop") {
			desktopFile = path
			return fs.SkipAll
		}
		return nil
	})
	if desktopFile == "" {
		return nil, fmt.Errorf("no .desktop file found in %s", root)
	}

	fh, err := os.Open(desktopFile)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	df, err := Parse(fh)
	if err != nil {
		return nil, err
	}

	iconName, _ := df.Desktop().Icon()
	if iconName == "" {
		return df, nil
	}

	iconSrc, _ := findIcon(root, iconName)

	if iconSrc != "" {
		iconDst := filepath.Join(filepath.Dir(appImagePath), filepath.Base(iconSrc))
		if err := copyFile(iconSrc, iconDst); err == nil {
			entry := df.Data["Desktop Entry"]
			entry["Icon"] = iconDst
		}
	}

	return df, nil
}

// copyFile copies src → dst (overwrites, keeps mode bits)
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	fi, _ := in.Stat()
	return os.Chmod(dst, fi.Mode())
}


// findIcon searches under root for files named iconName with a permitted extension.
// Example: Icon=obsidian → matches obsidian.svg, obsidian.png, ...
func findIcon(root, iconName string) (string, error) {
	extOrder := []string{".svg", ".png", ".xpm", ".ico", ".jpg", ".jpeg"}
	allowed := make(map[string]int, len(extOrder))
	for i, e := range extOrder {
		allowed[strings.ToLower(e)] = i
	}

	iconNameLower := strings.ToLower(iconName)
	var (
		bestPath string
		bestRank = 1<<30
	)

	errStop := errors.New("stop-walk")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		rank, ok := allowed[ext]
		if !ok {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if strings.ToLower(base) != iconNameLower {
			return nil
		}
		// Keep the best-ranked extension (e.g., prefer .svg over .png)
		if rank < bestRank {
			bestRank, bestPath = rank, path
			// Optional: stop immediately on first acceptable hit
			// return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return "", err
	}
	if bestPath == "" {
		return "", fmt.Errorf("icon %q not found under %s", iconName, root)
	}
	return bestPath, nil
}

package domain

import (
	"fmt"
	"strings"
	"unicode"
)

type DesktopEntry struct {
	Name    string
	Exec    string
	Icon    string
	Version Version
	Fields  map[string]string

	lines        []desktopEntryLine
	finalNewline bool
}

type desktopEntryLine struct {
	raw   string
	group string
	key   string
}

// ParseDesktopEntry parses a freedesktop .desktop file into a DesktopEntry.
//
// Only keys in the [Desktop Entry] group are mapped to the domain entry. Unknown
// keys are preserved in Fields so callers can later serialize the entry without
// losing metadata they do not understand yet.
func ParseDesktopEntry(content []byte) (DesktopEntry, error) {
	fields := make(map[string]string)
	lines := splitDesktopEntryLines(string(content))
	inDesktopEntry := false
	seenDesktopEntry := false
	currentGroup := ""

	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine.raw)
		entryLine := desktopEntryLine{
			raw:   rawLine.raw,
			group: currentGroup,
		}

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			lines[i] = entryLine
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentGroup = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			inDesktopEntry = currentGroup == "Desktop Entry"
			if inDesktopEntry {
				seenDesktopEntry = true
			}
			entryLine.group = currentGroup
			lines[i] = entryLine
			continue
		}

		entryLine.group = currentGroup
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			if inDesktopEntry {
				return DesktopEntry{}, fmt.Errorf("parse desktop entry line %q: missing '='", line)
			}
			lines[i] = entryLine
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			if inDesktopEntry {
				return DesktopEntry{}, fmt.Errorf("parse desktop entry line %q: empty key", line)
			}
			lines[i] = entryLine
			continue
		}

		entryLine.key = key
		if inDesktopEntry {
			fields[key] = value
		}
		lines[i] = entryLine
	}
	if !seenDesktopEntry {
		return DesktopEntry{}, fmt.Errorf("parse desktop entry: missing [Desktop Entry] group")
	}

	entry := DesktopEntry{
		Name:         desktopEntryName(fields),
		Exec:         fields["Exec"],
		Icon:         fields["Icon"],
		Fields:       fields,
		lines:        lines,
		finalNewline: len(content) > 0 && content[len(content)-1] == '\n',
	}
	if version, ok := ParseVersion(desktopEntryVersion(fields)); ok {
		entry.Version = version
	}

	return entry, nil
}

// WithExec returns a copy of the entry with the Desktop Entry Exec key updated.
func (d DesktopEntry) WithExec(exec string) DesktopEntry {
	return d.withField("Exec", exec)
}

// WithIcon returns a copy of the entry with the Desktop Entry Icon key updated.
func (d DesktopEntry) WithIcon(icon string) DesktopEntry {
	return d.withField("Icon", icon)
}

// Bytes serializes the desktop entry while preserving raw comments, groups, and
// field ordering from the parsed input. Mutated fields are rewritten as key=value.
func (d DesktopEntry) Bytes() []byte {
	lines := d.lines
	if len(lines) == 0 {
		lines = desktopEntryLinesFromFields(d.Fields)
	}

	serialized := make([]string, len(lines))
	for i, line := range lines {
		serialized[i] = line.raw
	}

	result := strings.Join(serialized, "\n")
	if d.finalNewline || len(serialized) > 0 {
		result += "\n"
	}

	return []byte(result)
}

func (d DesktopEntry) withField(key string, value string) DesktopEntry {
	value = strings.TrimSpace(value)
	updated := d
	updated.Fields = copyDesktopEntryFields(d.Fields)
	updated.Fields[key] = value
	updated.lines = copyDesktopEntryLines(d.lines)
	updated.setProjectedField(key, value)
	updated.upsertRawDesktopEntryField(key, value)
	if key == "Exec" {
		updated.rewriteDesktopActionExecFields(value)
	}
	return updated
}

func (d *DesktopEntry) setProjectedField(key string, value string) {
	switch key {
	case "Name":
		d.Name = value
	case "Exec":
		d.Exec = value
	case "Icon":
		d.Icon = value
	case "Version", "X-AppImage-Version":
		if version, ok := ParseVersion(value); ok {
			d.Version = version
		} else {
			d.Version = Version{}
		}
	}
}

func (d *DesktopEntry) rewriteDesktopActionExecFields(exec string) {
	for i, line := range d.lines {
		if !isDesktopActionExecLine(line) {
			continue
		}

		_, oldValue, ok := strings.Cut(strings.TrimSpace(line.raw), "=")
		if !ok {
			continue
		}
		d.lines[i].raw = "Exec=" + exec + desktopActionExecArgumentSuffix(strings.TrimSpace(oldValue))
	}
}

func desktopActionExecArgumentSuffix(exec string) string {
	index := strings.IndexFunc(exec, unicode.IsSpace)
	if index == -1 {
		return ""
	}

	return exec[index:]
}

func isDesktopActionExecLine(line desktopEntryLine) bool {
	return strings.HasPrefix(line.group, "Desktop Action ") && line.key == "Exec"
}

func (d *DesktopEntry) upsertRawDesktopEntryField(key string, value string) {
	if len(d.lines) == 0 {
		d.lines = desktopEntryLinesFromFields(d.Fields)
		d.finalNewline = true
		return
	}

	insertAt := len(d.lines)
	inDesktopEntry := false
	seenDesktopEntry := false
	for i, line := range d.lines {
		if isDesktopEntryGroupLine(line) {
			inDesktopEntry = true
			seenDesktopEntry = true
			insertAt = i + 1
			continue
		}
		if isGroupLine(line) && inDesktopEntry {
			insertAt = i
			break
		}
		if !inDesktopEntry {
			continue
		}
		if line.key == key {
			d.lines[i].raw = key + "=" + value
			return
		}
		insertAt = i + 1
	}

	newLine := desktopEntryLine{
		raw:   key + "=" + value,
		group: "Desktop Entry",
		key:   key,
	}
	if !seenDesktopEntry {
		d.lines = append([]desktopEntryLine{{raw: "[Desktop Entry]", group: "Desktop Entry"}, newLine}, d.lines...)
		d.finalNewline = true
		return
	}

	d.lines = append(d.lines, desktopEntryLine{})
	copy(d.lines[insertAt+1:], d.lines[insertAt:])
	d.lines[insertAt] = newLine
}

func desktopEntryName(fields map[string]string) string {
	if name := fields["X-AppImage-Name"]; name != "" {
		return name
	}

	return fields["Name"]
}

func desktopEntryVersion(fields map[string]string) string {
	if version := fields["X-AppImage-Version"]; version != "" {
		return version
	}

	return fields["Version"]
}

func splitDesktopEntryLines(content string) []desktopEntryLine {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}

	rawLines := strings.Split(content, "\n")
	lines := make([]desktopEntryLine, len(rawLines))
	for i, rawLine := range rawLines {
		lines[i] = desktopEntryLine{raw: strings.TrimSuffix(rawLine, "\r")}
	}

	return lines
}

func desktopEntryLinesFromFields(fields map[string]string) []desktopEntryLine {
	lines := []desktopEntryLine{{raw: "[Desktop Entry]", group: "Desktop Entry"}}
	for _, key := range []string{"Type", "Name", "Exec", "Icon", "Version", "X-AppImage-Name", "X-AppImage-Version"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		lines = append(lines, desktopEntryLine{
			raw:   key + "=" + value,
			group: "Desktop Entry",
			key:   key,
		})
	}

	return lines
}

func copyDesktopEntryFields(fields map[string]string) map[string]string {
	result := make(map[string]string, len(fields))
	for key, value := range fields {
		result[key] = value
	}

	return result
}

func copyDesktopEntryLines(lines []desktopEntryLine) []desktopEntryLine {
	result := make([]desktopEntryLine, len(lines))
	copy(result, lines)
	return result
}

func isDesktopEntryGroupLine(line desktopEntryLine) bool {
	return strings.TrimSpace(line.raw) == "[Desktop Entry]"
}

func isGroupLine(line desktopEntryLine) bool {
	raw := strings.TrimSpace(line.raw)
	return strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]")
}

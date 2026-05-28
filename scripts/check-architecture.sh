#!/bin/sh
set -eu

packages_file="$(mktemp)"
violations_file="$(mktemp)"
trap 'rm -f "$packages_file" "$violations_file"' EXIT

append_violation() {
	printf '%s\n' "$1" >> "$violations_file"
}

check_layer_imports() {
	go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}' ./internal/... > "$packages_file"

	while IFS= read -r line; do
		pkg=${line%% *}
		imports=${line#"$pkg"}
		forbidden=""

		case "$pkg" in
			*/internal/domain | */internal/domain/*)
				forbidden="/internal/app /internal/infra /internal/cli"
				;;
			*/internal/app | */internal/app/*)
				forbidden="/internal/infra /internal/cli"
				;;
			*/internal/infra | */internal/infra/*)
				forbidden="/internal/cli"
				;;
			*)
				continue
				;;
		esac

		for imported in $imports; do
			for boundary in $forbidden; do
				case "$imported" in
					*"$boundary"*)
						append_violation "$pkg imports $imported"
						;;
				esac
			done
		done
	done < "$packages_file"
}

check_cli_infra_imports() {
	for path in internal/cli/*.go; do
		[ -e "$path" ] || continue
		case "$path" in
			*_test.go | internal/cli/runtime.go | internal/cli/runtime_wiring.go)
				continue
				;;
		esac

		if grep -q '"[^"]*/internal/infra' "$path"; then
			grep '"[^"]*/internal/infra' "$path" | while IFS= read -r import_line; do
				append_violation "$path imports concrete infra outside runtime composition: $import_line"
			done
		fi
	done
}

check_layer_imports
check_cli_infra_imports

if [ -s "$violations_file" ]; then
	printf 'Architecture boundary violations:\n'
	cat "$violations_file"
	exit 1
fi

printf 'Architecture boundaries OK\n'

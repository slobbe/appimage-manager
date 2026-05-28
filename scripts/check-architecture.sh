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

is_allowed_cli_boundary_migration_file() {
	case "$1" in
		internal/cli/commands.go | \
		internal/cli/output.go | \
		internal/cli/package_sources.go | \
		internal/cli/update_workflow.go)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

is_forbidden_cli_boundary_import() {
	case "$1" in
		*'/internal/domain' | *'/internal/domain/'*)
			return 0
			;;
		*'/internal/infra' | *'/internal/infra/'*)
			return 0
			;;
		*'/internal/app/services' | *'/internal/app/services/'*)
			return 1
			;;
		*'/internal/app' | *'/internal/app/'*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

check_cli_boundary_imports() {
	for path in internal/cli/*.go; do
		[ -e "$path" ] || continue
		case "$path" in
			*_test.go | internal/cli/runtime.go | internal/cli/runtime_wiring.go)
				continue
				;;
		esac

		grep '"[^"]*/internal/\(app\|domain\|infra\)' "$path" | while IFS= read -r import_line; do
			import_path=${import_line#*\"}
			import_path=${import_path%%\"*}
			if ! is_forbidden_cli_boundary_import "$import_path"; then
				continue
			fi
			if is_allowed_cli_boundary_migration_file "$path"; then
				continue
			fi
			append_violation "$path imports outside the CLI app-service boundary: $import_line"
		done || true
	done
}

check_layer_imports
check_cli_boundary_imports

if [ -s "$violations_file" ]; then
	printf 'Architecture boundary violations:\n'
	cat "$violations_file"
	exit 1
fi

printf 'Architecture boundaries OK\n'

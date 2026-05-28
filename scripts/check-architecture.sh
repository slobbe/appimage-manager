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

is_allowed_cli_boundary_migration_import() {
	path=$1
	import_path=$2

	case "$path" in
		internal/cli/commands.go)
			case "$import_path" in
				*'/internal/app/update' | *'/internal/app/upgrade' | *'/internal/domain') return 0 ;;
			esac
			;;
		internal/cli/output.go)
			case "$import_path" in
				*'/internal/domain') return 0 ;;
			esac
			;;
		internal/cli/package_sources.go)
			case "$import_path" in
				*'/internal/app/discovery' | *'/internal/domain') return 0 ;;
			esac
			;;
		internal/cli/update_workflow.go)
			case "$import_path" in
				*'/internal/app/clock' | *'/internal/app/integrate' | *'/internal/app/update' | *'/internal/domain') return 0 ;;
			esac
			;;
	esac

	return 1
}

is_allowed_cli_test_boundary_migration_import() {
	path=$1
	import_path=$2

	case "$path" in
		internal/cli/commands_test.go)
			case "$import_path" in
				*'/internal/app/appimage' | \
				*'/internal/app/discovery' | \
				*'/internal/app/integrate' | \
				*'/internal/app/update' | \
				*'/internal/app/upgrade' | \
				*'/internal/domain' | \
				*'/internal/infra/config' | \
				*'/internal/infra/download' | \
				*'/internal/infra/filesystem' | \
				*'/internal/infra/repository') return 0 ;;
			esac
			;;
	esac

	return 1
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
			if is_allowed_cli_boundary_migration_import "$path" "$import_path"; then
				continue
			fi
			append_violation "$path imports outside the CLI app-service boundary: $import_line"
		done || true
	done
}

check_cli_test_boundary_imports() {
	for path in internal/cli/*_test.go; do
		[ -e "$path" ] || continue

		grep '"[^"]*/internal/\(app\|domain\|infra\)' "$path" | while IFS= read -r import_line; do
			import_path=${import_line#*\"}
			import_path=${import_path%%\"*}
			if ! is_forbidden_cli_boundary_import "$import_path"; then
				continue
			fi
			if is_allowed_cli_test_boundary_migration_import "$path" "$import_path"; then
				continue
			fi
			append_violation "$path imports outside the CLI test app-service boundary: $import_line"
		done || true
	done
}

check_layer_imports
check_cli_boundary_imports
check_cli_test_boundary_imports

if [ -s "$violations_file" ]; then
	printf 'Architecture boundary violations:\n'
	cat "$violations_file"
	exit 1
fi

printf 'Architecture boundaries OK\n'

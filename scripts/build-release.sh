#!/bin/sh

set -eu

version="${1:-dev}"
output_dir="${2:-dist}"
package="./cmd/tls-fetch-mcp"
temp_dir="$(mktemp -d)"

trap 'rm -rf "$temp_dir"' EXIT

case "$output_dir" in
	/*) ;;
	*) output_dir="$(pwd)/$output_dir" ;;
esac

mkdir -p "$output_dir"

build_archive() {
	goos="$1"
	goarch="$2"
	format="$3"
	name="tls-fetch-mcp_${version}_${goos}_${goarch}"
	binary="tls-fetch-mcp"

	if [ "$goos" = "windows" ]; then
		binary="${binary}.exe"
	fi

	build_dir="${temp_dir}/${name}"
	mkdir -p "$build_dir"

	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -trimpath \
		-ldflags "-s -w -X main.version=${version}" \
		-o "${build_dir}/${binary}" \
		"$package"

	cp CHANGELOG.md LICENSE README.md "$build_dir/"

	if [ "$format" = "zip" ]; then
		(
			cd "$build_dir"
			zip -q -r "${output_dir}/${name}.zip" .
		)
	else
		tar -C "$build_dir" -czf "${output_dir}/${name}.tar.gz" .
	fi
}

build_archive linux amd64 tar
build_archive linux arm64 tar
build_archive darwin amd64 tar
build_archive darwin arm64 tar
build_archive windows amd64 zip
build_archive windows arm64 zip

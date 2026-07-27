#!/bin/sh

set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT

version="1.2.3"
checksums_file="${temporary_dir}/checksums.txt"
output_file="${temporary_dir}/tls-fetch-mcp.rb"

printf '%s  %s\n' \
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
	"./tls-fetch-mcp_${version}_darwin_amd64.tar.gz" \
	"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" \
	"./tls-fetch-mcp_${version}_darwin_arm64.tar.gz" \
	"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" \
	"./tls-fetch-mcp_${version}_linux_amd64.tar.gz" \
	"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" \
	"./tls-fetch-mcp_${version}_linux_arm64.tar.gz" \
	>"$checksums_file"

sh "${script_dir}/render-homebrew-formula.sh" \
	"$version" \
	"$checksums_file" \
	"$output_file"

grep -Fq 'version "1.2.3"' "$output_file"
grep -Fq \
	'https://github.com/JakobAIOdev/tls-fetch-mcp/releases/download/v1.2.3/tls-fetch-mcp_1.2.3_darwin_arm64.tar.gz' \
	"$output_file"
grep -Fq \
	'sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"' \
	"$output_file"
grep -Fq \
	'assert_match "tls-fetch-mcp 1.2.3"' \
	"$output_file"

if grep -Eq '@@[A-Z0-9_]+@@' "$output_file"; then
	echo "rendered formula contains unresolved placeholders" >&2
	exit 1
fi

if sh "${script_dir}/render-homebrew-formula.sh" \
	"$version" \
	"${temporary_dir}/missing-checksums.txt" \
	"${temporary_dir}/should-not-exist.rb" \
	2>/dev/null; then
	echo "renderer unexpectedly accepted a missing checksums file" >&2
	exit 1
fi

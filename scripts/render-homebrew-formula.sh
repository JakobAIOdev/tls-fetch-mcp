#!/bin/sh

set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: $0 VERSION CHECKSUMS_FILE OUTPUT_FILE" >&2
	exit 2
fi

version="$1"
checksums_file="$2"
output_file="$3"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
template_file="${script_dir}/../packaging/homebrew/tls-fetch-mcp.rb.tmpl"

case "$version" in
	"" | *[!0-9A-Za-z.+-]*)
		echo "invalid release version: $version" >&2
		exit 2
		;;
esac

if [ ! -f "$checksums_file" ]; then
	echo "checksums file does not exist: $checksums_file" >&2
	exit 2
fi

checksum_for() {
	artifact="$1"
	checksum="$(awk -v target="./${artifact}" '$2 == target { print $1 }' "$checksums_file")"

	if [ "${#checksum}" -ne 64 ] || printf '%s' "$checksum" | grep -Eq '[^0-9a-f]'; then
		echo "missing or invalid SHA-256 for ${artifact}" >&2
		exit 1
	fi

	printf '%s' "$checksum"
}

darwin_amd64_sha256="$(
	checksum_for "tls-fetch-mcp_${version}_darwin_amd64.tar.gz"
)"
darwin_arm64_sha256="$(
	checksum_for "tls-fetch-mcp_${version}_darwin_arm64.tar.gz"
)"
linux_amd64_sha256="$(
	checksum_for "tls-fetch-mcp_${version}_linux_amd64.tar.gz"
)"
linux_arm64_sha256="$(
	checksum_for "tls-fetch-mcp_${version}_linux_arm64.tar.gz"
)"

output_dir="$(dirname -- "$output_file")"
mkdir -p "$output_dir"
temporary_file="$(mktemp "${output_dir}/.tls-fetch-mcp.rb.XXXXXX")"
trap 'rm -f "$temporary_file"' EXIT

sed \
	-e "s|@@VERSION@@|${version}|g" \
	-e "s|@@DARWIN_AMD64_SHA256@@|${darwin_amd64_sha256}|g" \
	-e "s|@@DARWIN_ARM64_SHA256@@|${darwin_arm64_sha256}|g" \
	-e "s|@@LINUX_AMD64_SHA256@@|${linux_amd64_sha256}|g" \
	-e "s|@@LINUX_ARM64_SHA256@@|${linux_arm64_sha256}|g" \
	"$template_file" >"$temporary_file"

if grep -Eq '@@[A-Z0-9_]+@@' "$temporary_file"; then
	echo "unresolved placeholder in rendered Homebrew formula" >&2
	exit 1
fi

mv "$temporary_file" "$output_file"
chmod 0644 "$output_file"
trap - EXIT

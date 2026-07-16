#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: render_homebrew_formula.sh VERSION CHECKSUMS OUTPUT" >&2
  exit 2
fi

version=${1#v}
checksums=$2
output=$3

checksum() {
  name="rolloutviz_${version}_$1_$2.tar.gz"
  value=$(awk -v name="$name" '$2 == name { print $1 }' "$checksums")
  if [ -z "$value" ]; then
    echo "missing checksum for $name" >&2
    exit 1
  fi
  printf '%s' "$value"
}

darwin_amd64=$(checksum darwin x86_64)
darwin_arm64=$(checksum darwin arm64)
linux_amd64=$(checksum linux x86_64)
linux_arm64=$(checksum linux arm64)

sed \
  -e "s/@VERSION@/$version/g" \
  -e "s/@DARWIN_AMD64@/$darwin_amd64/g" \
  -e "s/@DARWIN_ARM64@/$darwin_arm64/g" \
  -e "s/@LINUX_AMD64@/$linux_amd64/g" \
  -e "s/@LINUX_ARM64@/$linux_arm64/g" \
  "$(dirname "$0")/../packaging/homebrew/rolloutviz.rb.tmpl" > "$output"

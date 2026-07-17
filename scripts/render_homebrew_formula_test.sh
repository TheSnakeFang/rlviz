#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

for target in "darwin x86_64" "darwin arm64" "linux x86_64" "linux arm64"; do
  set -- $target
  os=$1
  arch=$2
  digest=$(printf '%064d' "${#target}")
  printf '%s  rlviz_0.1.0_%s_%s.tar.gz\n' "$digest" "$os" "$arch" >> "$temporary/checksums.txt"
done

"$root/scripts/render_homebrew_formula.sh" v0.1.0 "$temporary/checksums.txt" "$temporary/rlviz.rb"
grep -q 'version "0.1.0"' "$temporary/rlviz.rb"
grep -q 'rlviz_0.1.0_darwin_arm64.tar.gz' "$temporary/rlviz.rb"
grep -q 'class Rlviz < Formula' "$temporary/rlviz.rb"

if "$root/scripts/render_homebrew_formula.sh" 0.2.0 "$temporary/checksums.txt" "$temporary/missing.rb" 2>/dev/null; then
  echo "expected missing checksums to fail" >&2
  exit 1
fi

echo "homebrew formula tests passed"

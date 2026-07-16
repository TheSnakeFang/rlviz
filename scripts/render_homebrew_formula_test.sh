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
  printf '%s  rolloutviz_0.1.0_%s_%s.tar.gz\n' "$digest" "$os" "$arch" >> "$temporary/checksums.txt"
done

"$root/scripts/render_homebrew_formula.sh" v0.1.0 "$temporary/checksums.txt" "$temporary/rolloutviz.rb"
grep -q 'version "0.1.0"' "$temporary/rolloutviz.rb"
grep -q 'rolloutviz_0.1.0_darwin_arm64.tar.gz' "$temporary/rolloutviz.rb"
grep -q 'bin.install_symlink "rlviz" => "rolloutviz"' "$temporary/rolloutviz.rb"

if "$root/scripts/render_homebrew_formula.sh" 0.2.0 "$temporary/checksums.txt" "$temporary/missing.rb" 2>/dev/null; then
  echo "expected missing checksums to fail" >&2
  exit 1
fi

echo "homebrew formula tests passed"

#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source_dir="$script_dir/AI-Setup-Helper"
output_zip="$script_dir/AI-Setup-Helper.zip"

if [[ ! -d "$source_dir" ]]; then
	echo "AI-Setup-Helper directory not found: $source_dir" >&2
	exit 1
fi

rm -f "$output_zip"
cd "$source_dir"
zip -r "$output_zip" .

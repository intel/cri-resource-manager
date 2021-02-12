#!/bin/bash -e

set -o pipefail

tmpfile="_sst_types_out.go"
trap "rm -f $tmpfile" EXIT

generate() {
    local target="$1"
    shift
    local copts=$@

    echo "Generating $target..."

    go tool cgo -godefs -- $copts _"$target" | gofmt > "$tmpfile"
    mv "$tmpfile" "$target"
}

KERNEL_SRC_DIR="${KERNEL_SRC_DIR:-/usr/src/linux}"

echo "INFO: using kernel sources at $KERNEL_SRC_DIR"

# Generate types from Linux kernel (public) headers
generate sst_types_amd64.go -I"$KERNEL_SRC_DIR/include/uapi" "-I$KERNEL_SRC_DIR/include"

# Generate types from Linux kernel private headers
generate sst_types_priv.go -I"$KERNEL_SRC_DIR"

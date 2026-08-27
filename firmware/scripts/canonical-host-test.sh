#!/usr/bin/env bash
# =============================================================================
# Host test string kanonik firmware (test/canonical_host_test.cpp).
#
# src/canonical.cpp sengaja tidak bergantung pada Arduino maupun mbedTLS, jadi
# g++ biasa dapat mengompilasinya. Yang diperiksa adalah hal yang tidak dapat
# diperiksa dari sisi server: bahwa firmware membangun string yang SAMA
# byte-per-byte dengan yang diharapkan server Go.
#
#   ./scripts/canonical-host-test.sh     # dari direktori firmware/
#
# Keluar 0 bila semua vektor cocok, 1 bila ada yang tidak.
# =============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

# CANONICAL_BUFFER_SIZE dibaca dari config.h, bukan disalin ke test: satu sumber
# nilai berarti mengecilkan buffer di config.h akan MENGGAGALKAN test, bukan
# lolos tanpa diketahui.
size=$(sed -n 's/^#define[[:space:]]\+CANONICAL_BUFFER_SIZE[[:space:]]\+\([0-9]\+\).*/\1/p' src/config.h)
if [ -z "$size" ]; then
    echo "tidak dapat membaca CANONICAL_BUFFER_SIZE dari src/config.h" >&2
    exit 1
fi

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

g++ -std=c++17 -Wall -Wextra -Werror -O1 \
    -DCANONICAL_BUFFER_SIZE="$size" \
    -o "$out/canonical_host_test" \
    test/canonical_host_test.cpp src/canonical.cpp

"$out/canonical_host_test"

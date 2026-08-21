# =============================================================================
# QuakeAlert Server — image produksi
#
# Multi-stage: toolchain Go dipakai untuk build lalu dibuang, sehingga image
# akhir hanya berisi satu binary statis di atas base tanpa shell. Target deploy
# adalah VPS 1 vCPU / 1 GB (ADR-0001) — image kecil bukan soal estetika: ia
# menentukan berapa lama sebuah node yang mati butuh untuk kembali menerima
# paket sensor.
#
# Build (dari root repo):
#   docker build -f server/Dockerfile -t quakealert-server:latest server
# =============================================================================

# --- Stage 1: build ----------------------------------------------------------
# Versi Go dipin (bukan :latest) agar build hari ini dan build enam bulan lagi
# menghasilkan binary yang sama; go.mod menuntut >= 1.24.0.
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Dependency di-copy lebih dulu supaya layer `go mod download` hanya batal
# ketika go.mod/go.sum berubah, bukan setiap kali satu baris kode disentuh.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0: binary statis, syarat agar bisa jalan di base tanpa libc.
# -trimpath membuang path absolut mesin build; -w -s membuang DWARF + tabel
# simbol (± 30% ukuran) — panic trace tetap menyebutkan fungsi dan baris.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-w -s" \
        -o /out/quakealert \
        ./cmd/quakealert

# --- Stage 2: runtime --------------------------------------------------------
# distroless static: tanpa shell, tanpa package manager, tanpa busybox. Sebuah
# RCE di sini tidak menemukan `sh` untuk dieksekusi. Bawaannya sudah termasuk
# CA root (dibutuhkan FCM HTTP v1 + MQTTS) dan /etc/passwd berisi user nonroot.
FROM gcr.io/distroless/static-debian12:nonroot

# Zona waktu tidak ikut di distroless static; server bekerja dalam UTC dan epoch
# millis di seluruh kontrak, jadi ini dinyatakan eksplisit ketimbang diwarisi.
ARG VERSION=dev
LABEL org.opencontainers.image.title="QuakeAlert Server" \
      org.opencontainers.image.source="https://github.com/banana-pixel/quakealert" \
      org.opencontainers.image.version="${VERSION}"

ARG VERSION=dev
LABEL org.opencontainers.image.title="QuakeAlert Server" \
      org.opencontainers.image.source="https://github.com/banana-pixel/quakealert" \
      org.opencontainers.image.version="${VERSION}"

ENV TZ=UTC

COPY --from=builder /out/quakealert /usr/local/bin/quakealert

# UID 65532 (nonroot). Bind ke :8080 — port tinggi, jadi tidak perlu root sama
# sekali; TLS diterminasi reverse proxy di depan (lihat docker-compose.prod.yml).
USER nonroot:nonroot
EXPOSE 8080

# Healthcheck TIDAK didefinisikan di sini: image tanpa shell juga tanpa
# curl/wget, dan menambahkan salah satunya membatalkan alasan memakai
# distroless. Compose memeriksa /healthz dari sisi reverse proxy.
ENTRYPOINT ["/usr/local/bin/quakealert"]

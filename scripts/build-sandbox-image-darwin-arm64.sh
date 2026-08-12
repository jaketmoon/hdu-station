#!/bin/sh
set -eu

# Release tooling only. Station users receive the signed, verified raw disk;
# QEMU and this builder are never bundled into the desktop application.

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
  echo "build-sandbox-image-darwin-arm64 requires macOS arm64" >&2
  exit 1
fi

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
output_dir=${HDU_STATION_SANDBOX_OUTPUT_DIR:-"$project_dir/dist/sandbox"}
base_url=${HDU_STATION_DEBIAN_IMAGE_URL:-https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-arm64.qcow2}
base_sha512=${HDU_STATION_DEBIAN_IMAGE_SHA512:-0ddd6ae10dad18535fc8a8167065e78565a04b721cae3e946a3ca4fda2ce54ac7a7546a09b9c0bca6bd101db3ab950056da19dc452113631dc7bbce7c96a404f}
vfkit_path=${HDU_STATION_VFKIT_PATH:-"$HOME/Library/Application Support/HDU Station/tools/vfkit/v0.6.4/vfkit"}
work_dir=${HDU_STATION_SANDBOX_WORK_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/hdu-station-sandbox.XXXXXX")}
vfkit_pid=

cleanup() {
  if [ -n "$vfkit_pid" ] && kill -0 "$vfkit_pid" 2>/dev/null; then
    kill -INT "$vfkit_pid" 2>/dev/null || true
    wait "$vfkit_pid" 2>/dev/null || true
  fi
  if [ -z "${HDU_STATION_SANDBOX_WORK_DIR:-}" ]; then
    rm -rf "$work_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required release-build tool: $1" >&2
    exit 1
  }
}

require curl
require shasum
require qemu-img
require base64
require make
require go

if [ ! -x "$vfkit_path" ]; then
  echo "verified Station vfkit sidecar is required at $vfkit_path" >&2
  exit 1
fi
if [ -z "${HDU_STATION_IMAGE_SIGNING_KEY:-}" ] || [ ! -f "$HDU_STATION_IMAGE_SIGNING_KEY" ]; then
  echo "HDU_STATION_IMAGE_SIGNING_KEY must point to an Ed25519 private key" >&2
  exit 1
fi
mkdir -p "$output_dir"
mkdir -p "$work_dir"
make -C "$project_dir" sandbox-runtime
make -C "$project_dir" sandbox-image-metadata

base_image="$work_dir/debian-arm64.qcow2"
base_partial="$work_dir/debian-arm64.qcow2.part"
runtime="$project_dir/build/bin/hdu-station-runtime-linux-arm64"
runtime_probe="$work_dir/hdu-station-runtime-probe"
metadata_signer="$project_dir/build/bin/hdu-station-sandbox-image-metadata"
seed_dir="$work_dir/seed"
raw_image="$work_dir/hdu-station-runtime-darwin-arm64.raw"
runtime_socket="$work_dir/runtime.sock"
efi_store="$work_dir/efi-variables.fd"
vfkit_log="$work_dir/vfkit.log"
guest_serial_log="$work_dir/guest-serial.log"
final_image="$output_dir/hdu-station-runtime-darwin-arm64.raw"
metadata="$output_dir/hdu-station-runtime-darwin-arm64.json"
go build -trimpath -o "$runtime_probe" "$project_dir/cmd/sandbox-runtime-probe"

if [ ! -f "$base_image" ] || [ "$(shasum -a 512 "$base_image" | awk '{print $1}')" != "$base_sha512" ]; then
  rm -f "$base_image"
  curl --fail --location --proto '=https' --tlsv1.2 \
    --continue-at - --retry 5 --retry-all-errors --connect-timeout 20 \
    --output "$base_partial" "$base_url"
  actual_base_sha512=$(shasum -a 512 "$base_partial" | awk '{print $1}')
  if [ "$actual_base_sha512" != "$base_sha512" ]; then
    echo "Debian base image SHA-512 mismatch" >&2
    exit 1
  fi
  mv "$base_partial" "$base_image"
fi
actual_base_sha512=$(shasum -a 512 "$base_image" | awk '{print $1}')
if [ "$actual_base_sha512" != "$base_sha512" ]; then
  echo "Debian base image SHA-512 mismatch" >&2
  exit 1
fi
qemu-img convert -f qcow2 -O raw "$base_image" "$raw_image"

runtime_base64=$(base64 < "$runtime" | tr -d '\n')
mkdir -p "$seed_dir"
cat > "$seed_dir/meta-data" <<'EOF'
instance-id: hdu-station-sandbox-v1
local-hostname: hdu-station-sandbox
EOF
cat > "$seed_dir/user-data" <<EOF
#cloud-config
output:
  all: '| tee -a /dev/hvc0'
write_files:
  - path: /usr/local/bin/hdu-station-runtime
    permissions: '0755'
    encoding: b64
    content: $runtime_base64
  - path: /etc/systemd/system/hdu-station-runtime.service
    permissions: '0644'
    content: |
      [Unit]
      Description=HDU Station sandbox runtime
      After=network.target
      [Service]
      # Some Debian arm64 kernels build vsock in rather than as a loadable
      # module. Do not turn a harmless "module not found" into a failed
      # runtime service; listenVsock still determines actual availability.
      ExecStartPre=-/usr/sbin/modprobe vsock
      ExecStart=/usr/local/bin/hdu-station-runtime --vsock-port 1024
      Restart=always
      RestartSec=1
      StandardOutput=journal+console
      StandardError=journal+console
      NoNewPrivileges=yes
      PrivateTmp=yes
      ProtectHome=yes
      ProtectSystem=full
      [Install]
      WantedBy=multi-user.target
runcmd:
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, hdu-station-runtime.service]
  - [sh, -c, 'sleep 3; systemctl status hdu-station-runtime.service --no-pager --full > /dev/hvc0 2>&1 || true; journalctl -u hdu-station-runtime.service --no-pager --full > /dev/hvc0 2>&1 || true']
EOF

# vfkit creates the NoCloud ISO itself from files named user-data/meta-data.
# The raw disk becomes self-contained after this one controlled initialization.
"$vfkit_path" \
  --log-level error \
  --cpus 2 \
  --memory 2048 \
  --bootloader "efi,variable-store=$efi_store,create" \
  --cloud-init "$seed_dir/user-data,$seed_dir/meta-data" \
  --device "virtio-blk,path=$raw_image" \
  --device "virtio-serial,logFilePath=$guest_serial_log" \
  --device virtio-net,nat \
  --device "virtio-vsock,port=1024,socketURL=$runtime_socket,connect" \
  --device virtio-rng >"$vfkit_log" 2>&1 &
vfkit_pid=$!

# An initialized guest must prove the actual Station protocol, not merely boot.
deadline=$(( $(date +%s) + 180 ))
while :; do
  if [ -S "$runtime_socket" ]; then
    if "$runtime_probe" --socket "$runtime_socket" --op ping --timeout 2s >/dev/null 2>&1; then
      break
    fi
  fi
  if ! kill -0 "$vfkit_pid" 2>/dev/null; then
    echo "vfkit exited before the guest runtime became ready; log follows:" >&2
    cat "$vfkit_log" >&2 || true
    exit 1
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "timed out waiting for the guest runtime ping" >&2
    exit 1
  fi
  sleep 1
done
if ! "$runtime_probe" --socket "$runtime_socket" --op execute --command printf --arg sandbox-ok --timeout 10s | grep -q 'sandbox-ok'; then
  echo "guest runtime command verification failed" >&2
  exit 1
fi

kill -INT "$vfkit_pid"
wait "$vfkit_pid" || true
vfkit_pid=

cp "$raw_image" "$final_image"
digest=$(shasum -a 256 "$final_image" | awk '{print $1}')
"$metadata_signer" \
  --private-key "$HDU_STATION_IMAGE_SIGNING_KEY" \
  --sha256 "$digest" \
  --platform darwin/arm64 \
  --file "$(basename "$final_image")" \
  --output "$metadata"
echo "$metadata"

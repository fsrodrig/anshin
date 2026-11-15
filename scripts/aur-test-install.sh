#!/usr/bin/env bash
# Test the rendered package without downloading assets, so the package and release archives cannot drift.
set -euo pipefail

if [ $# -ne 1 ] || [ ! -f "$1/PKGBUILD" ]; then
  echo "usage: $0 <dir holding a rendered PKGBUILD and the archives it names>" >&2
  exit 2
fi
dir=$(cd "$1" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
image=$(sed -n 's/^FROM //p' "$root/deploy/aur/Dockerfile")

docker run --rm -i --label anshin=aur-test -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  -v "$dir:/pkg" -v "$root/deploy:/deploy:ro" "$image" bash -euo pipefail -s <<'EOF'
pacman -Syu --noconfirm --needed tailscale namcap > /dev/null
useradd --create-home --home-dir /build builder
install -d -o builder /build/pkg /build/template
install -o builder -m 644 /pkg/PKGBUILD /pkg/*.tar.gz /build/pkg/
install -o builder -m 644 /deploy/aur/PKGBUILD /build/template/
cd /build/template
if su builder -c 'makepkg --printsrcinfo' > /dev/null 2>&1; then
  echo "aur-test-install: makepkg accepted the unrendered template" >&2
  exit 1
fi
cd /build/pkg
su builder -c 'makepkg --printsrcinfo > .SRCINFO && makepkg --noconfirm'
namcap PKGBUILD anshin-agent-bin-*.pkg.tar.zst | tee /tmp/namcap
if grep -q ' E: ' /tmp/namcap; then
  echo "aur-test-install: namcap reports an error" >&2
  exit 1
fi
pacman -U --noconfirm anshin-agent-bin-*.pkg.tar.zst
diff <(pacman -Qlq anshin-agent-bin | grep -v '/$') - <<'LIST'
/usr/bin/anshin-agent
/usr/lib/systemd/system/anshin-agent.service
/usr/share/licenses/anshin-agent-bin/LICENSE
LIST
cmp /usr/lib/systemd/system/anshin-agent.service /deploy/systemd/anshin-agent.service
systemd-analyze verify /usr/lib/systemd/system/anshin-agent.service
version=$(sed -n 's/^pkgver=//p' PKGBUILD)
[ "$(anshin-agent version)" = "anshin-agent $version" ]
install -m 644 -o "$HOST_UID" -g "$HOST_GID" .SRCINFO /pkg/.SRCINFO
EOF

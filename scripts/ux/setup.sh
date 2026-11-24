#!/usr/bin/env bash
# Install the packages needed to run mailrat under Xvfb. Prints them if no root.
set -euo pipefail
pkgs=(xvfb xdotool x11-utils imagemagick scrot openbox x11vnc novnc websockify libgl1-mesa-dri libgl1
  libegl1 libxcursor1 libxrandr2 libxinerama1 libxi6 libxxf86vm1
  gcc libgl1-mesa-dev xorg-dev)
missing=()
for p in "${pkgs[@]}"; do
  dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p")
done
if [[ ${#missing[@]} -eq 0 ]]; then echo "all packages present"; exit 0; fi
if [[ $EUID -eq 0 ]]; then sudo=()
elif sudo -n true 2>/dev/null; then sudo=(sudo)
else echo "need root; run: apt-get install -y ${missing[*]}"; exit 1; fi
"${sudo[@]}" apt-get update
"${sudo[@]}" env DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}"

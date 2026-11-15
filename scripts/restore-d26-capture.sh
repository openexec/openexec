#!/bin/sh
# Restore the private D26 capture from an authentic copy, hash-verified.
# usage: restore-d26-capture.sh SRC [DEST]
# Refuses (exit 1) unless SHA-256(SRC) equals the required hash. Never prints content.
set -eu
WANT=0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708
SRC=${1:?usage: restore-d26-capture.sh SRC [DEST]}
DEST=${2:-/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json}
[ -f "$SRC" ] || { echo "refused: $SRC is not a file" >&2; exit 1; }
GOT=$(sha256sum "$SRC" | cut -d' ' -f1)
if [ "$GOT" != "$WANT" ]; then
  echo "refused: sha256 $GOT != required $WANT" >&2
  exit 1
fi
install -m 0600 "$SRC" "$DEST"
echo "restored $DEST (sha256 verified)"

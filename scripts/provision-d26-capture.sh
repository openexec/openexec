#!/bin/sh
# Install the D26 private capture only if its SHA-256 matches the recorded hash.
# Verify hash, then install; never synthesize. Never prints file contents.
#
# usage: provision-d26-capture.sh [--expected-sha256 HEX] [--dest PATH] CANDIDATE
# The two flags exist so the refusal and install paths can be exercised against
# throwaway files; real provisioning passes only CANDIDATE.
set -u

EXPECTED=0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708
DEST=/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json

while [ $# -gt 0 ]; do
	case "$1" in
	--expected-sha256)
		[ $# -ge 2 ] || { echo "refused: --expected-sha256 needs a value" >&2; exit 2; }
		EXPECTED=$2
		shift 2
		;;
	--dest)
		[ $# -ge 2 ] || { echo "refused: --dest needs a value" >&2; exit 2; }
		DEST=$2
		shift 2
		;;
	--)
		shift
		break
		;;
	-*)
		echo "refused: unknown flag" >&2
		exit 2
		;;
	*) break ;;
	esac
done
[ $# -eq 1 ] || { echo "usage: provision-d26-capture.sh [--expected-sha256 HEX] [--dest PATH] CANDIDATE" >&2; exit 2; }
CAND=$1

hash_of() { sha256sum "$1" | cut -d' ' -f1; }

if [ ! -f "$CAND" ] || [ ! -r "$CAND" ]; then
	echo "refused: candidate missing or unreadable; nothing installed" >&2
	exit 1
fi
GOT=$(hash_of "$CAND")
if [ "$GOT" != "$EXPECTED" ]; then
	echo "refused: candidate sha256 does not match expected $EXPECTED; nothing installed" >&2
	exit 1
fi
mkdir -p "$(dirname "$DEST")" || { echo "refused: cannot create destination directory" >&2; exit 1; }
TMP="$DEST.provision.$$"
if ! cp "$CAND" "$TMP" || [ "$(hash_of "$TMP")" != "$EXPECTED" ]; then
	rm -f "$TMP"
	echo "refused: copy failed verification; nothing installed" >&2
	exit 1
fi
mv "$TMP" "$DEST" || { rm -f "$TMP"; echo "refused: install failed" >&2; exit 1; }
echo "installed: sha256 verified $EXPECTED"
exit 0

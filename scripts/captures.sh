#!/usr/bin/env bash
# Render every gated capture test: one ebiten loop per process, so each test
# runs on its own. PNGs land in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
#
#   scripts/captures.sh          every capture
#   scripts/captures.sh HUD      the captures whose test name matches
#
# NEW CAPTURES MUST BE ADDED TO THE LIST BELOW: the gate lives in the driver,
# not in the test body, so this list is the only index of what exists — and a
# stale name fails the run (the PASS-line check below) instead of no-oping.
set -euo pipefail
cd "$(dirname "$0")/.."

captures() {
	cat <<'EOF'
./game TestDockedBackdropCapture
./game TestTitleCaptureFrame
./game TestTitleKeybindingsCapture
./game TestTitleCreditsCapture
./internal/capture TestCaptureLoopStaysHidden
./internal/capture TestCaptureWritesEvidenceOnVerifyFailure
./render TestAlphaCompositeGroundTruth
./uikit/screens TestFittingCaptureFrame
./uikit/screens TestHeaderCaptureFrame
./uikit/screens TestHudCaptureFrame
./uikit/screens TestIconsCaptureFrame
./uikit/screens TestInfoWindowCaptureFrame
./uikit/screens TestLogWindowCapture
./uikit/screens TestMapCaptureFrame
./uikit/screens TestOrderWindowCaptureFrame
./uikit/screens TestRefineryPickerCapture
./uikit/screens TestRailPixelsDrawAboveWindows
./uikit/screens TestSettingsCaptureFrame
./uikit/screens TestSplitPaneSeamCapture
./uikit/screens TestTooltipCaptureFrame
./uikit/screens TestWindowClipCapture
./uikit/screens TestWindowFadeAndTranslucencyCapture
EOF
}

export ENTIEVE_CAPTURE=1
export ENTIEVE_CAPTURE_DIR="${ENTIEVE_CAPTURE_DIR:-/tmp/opencode}"
mkdir -p "$ENTIEVE_CAPTURE_DIR"

filter="${1:-}"
ran=0
while read -r pkg name; do
	if [ -n "$filter" ] && [[ "$name" != *"$filter"* ]]; then
		continue
	fi
	printf '==> %s %s\n' "$pkg" "$name"
	go test "$pkg" -run "^$name\$" -count=1 -v >"/tmp/entieve-capture-$name.log" 2>&1 || {
		cat "/tmp/entieve-capture-$name.log" >&2
		printf 'capture FAILED: %s %s (log /tmp/entieve-capture-%s.log)\n' "$pkg" "$name" "$name" >&2
		exit 1
	}
	if ! grep -q -- "--- PASS: $name" "/tmp/entieve-capture-$name.log"; then
		printf 'capture ran nothing: %s %s (log /tmp/entieve-capture-%s.log)\n' "$pkg" "$name" "$name" >&2
		exit 1
	fi
	ran=$((ran + 1))
done < <(captures)

if [ "$ran" -eq 0 ]; then
	printf 'no capture matches %q\n' "$filter" >&2
	exit 2
fi
printf '%d captures written to %s\n' "$ran" "$ENTIEVE_CAPTURE_DIR"

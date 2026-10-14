// KWin 6 helper loaded by run-android-tv-emulator.sh.
//
// X11 geometry tools see Xwayland's synthetic coordinate space and can place an emulator mostly
// off-screen on a mixed-orientation Wayland desktop. KWin owns the real output geometry, so target
// the emulator by caption and center its frame on the primary output explicitly.
const target = workspace
    .windowList()
    .find((window) => window.caption.startsWith("Android Emulator - "));

if (target) {
    const bounds = workspace.screenOrder[0].geometry;
    const frame = target.frameGeometry;
    target.frameGeometry = {
        x: Math.round(bounds.x + (bounds.width - frame.width) / 2),
        y: Math.round(bounds.y + (bounds.height - frame.height) / 2),
        width: frame.width,
        height: frame.height,
    };
    workspace.activeWindow = target;
    console.info(
        `loomarr-center-android-tv: centered ${target.caption} at ` +
        `${target.frameGeometry.x},${target.frameGeometry.y} on ${workspace.screenOrder[0].name}`,
    );
}

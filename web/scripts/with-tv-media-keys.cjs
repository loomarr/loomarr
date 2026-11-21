const { withMainActivity } = require("@expo/config-plugins");

// The remote's Play/Pause reaches the app twice on Android TV (#3916): react-native-tvos hands it to
// JS from the root view (`useTVEventHandler`'s `playPause`, the Watching controls' own transport),
// and, because no view consumes it, Android's fallback then sends it to the app's media-button
// session. expo-video builds one for every player (`ExpoVideoBasicMediaSession_*`) and it toggles
// ExoPlayer on its own, so a press paused the chrome's clock and resumed the picture. Reporting the
// play keys as handled in the activity ends the fallback; JS already has them.

const MARKER = "// loomarr: the remote's play keys are the app's (#3916)";

const OVERRIDES = `
  ${MARKER}
  private fun isPlayKey(keyCode: Int): Boolean =
      keyCode == KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE ||
          keyCode == KeyEvent.KEYCODE_MEDIA_PLAY ||
          keyCode == KeyEvent.KEYCODE_MEDIA_PAUSE

  override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean =
      super.onKeyDown(keyCode, event) || isPlayKey(keyCode)

  override fun onKeyUp(keyCode: Int, event: KeyEvent): Boolean =
      super.onKeyUp(keyCode, event) || isPlayKey(keyCode)
`;

function keepPlayKeysInTheApp(source) {
  if (source.includes(MARKER)) return source;
  const classStart = source.search(/class MainActivity\s*:\s*ReactActivity\(\)\s*\{/);
  const classEnd = source.lastIndexOf("}");
  if (classStart === -1 || classEnd < classStart) {
    throw new Error("Could not find MainActivity's class body while keeping the remote's play keys");
  }
  const withImport = source.includes("import android.view.KeyEvent")
    ? source
    : source.replace(/^(package [^\n]+\n)/, "$1import android.view.KeyEvent\n");
  const end = withImport.lastIndexOf("}");
  return `${withImport.slice(0, end).trimEnd()}\n${OVERRIDES}}\n`;
}

function withTvMediaKeys(config) {
  return withMainActivity(config, (activityConfig) => {
    if (activityConfig.modResults.language !== "kt") {
      throw new Error("MainActivity is not Kotlin; the remote's play keys need a Kotlin activity");
    }
    activityConfig.modResults.contents = keepPlayKeysInTheApp(activityConfig.modResults.contents);
    return activityConfig;
  });
}

module.exports = withTvMediaKeys;
module.exports.keepPlayKeysInTheApp = keepPlayKeysInTheApp;

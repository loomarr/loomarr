const { withMainActivity } = require("@expo/config-plugins");
const { mergeContents } = require("@expo/config-plugins/build/utils/generateCode");

/**
 * Play/Pause on an Android TV remote belongs to Loomarr's own transport (#2088 decision 15). React
 * Native observes the key without consuming it, so Android then hands it to the media-button session
 * — expo-video's — which toggles the player a second time and undoes the viewer's pause. The main
 * activity consumes the key once React Native has seen it.
 */
const PLAY_PAUSE_OVERRIDE = `
  // Play/Pause is the app's own (#2088 decision 15): once React Native has seen it, keep it from the
  // media session, which would toggle the player again.
  override fun dispatchKeyEvent(event: KeyEvent): Boolean {
    val handled = super.dispatchKeyEvent(event)
    return event.keyCode == KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE || handled
  }
`;

const IMPORTS_ANCHOR = /^import android\.os\.Bundle$/m;
const CLASS_ANCHOR = /^class MainActivity : ReactActivity\(\) \{$/m;

function consumePlayPauseKey(source) {
  if (!IMPORTS_ANCHOR.test(source) || !CLASS_ANCHOR.test(source)) {
    throw new Error("Could not find Expo's MainActivity shape while applying the Play/Pause key");
  }
  const imported = mergeContents({
    anchor: IMPORTS_ANCHOR,
    comment: "//",
    newSrc: "import android.view.KeyEvent",
    offset: 1,
    src: source,
    tag: "loomarr-play-pause-key-import",
  });
  return mergeContents({
    anchor: CLASS_ANCHOR,
    comment: "//",
    newSrc: PLAY_PAUSE_OVERRIDE,
    offset: 1,
    src: imported.contents,
    tag: "loomarr-play-pause-key",
  }).contents;
}

function withTvPlayPauseKey(config) {
  return withMainActivity(config, (activityConfig) => {
    if (activityConfig.modResults.language !== "kt") {
      throw new Error("The Play/Pause key expects a Kotlin MainActivity");
    }
    activityConfig.modResults.contents = consumePlayPauseKey(activityConfig.modResults.contents);
    return activityConfig;
  });
}

module.exports = withTvPlayPauseKey;
module.exports.consumePlayPauseKey = consumePlayPauseKey;

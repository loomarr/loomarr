const assert = require("node:assert/strict");
const test = require("node:test");
const { consumePlayPauseKey } = require("./with-tv-play-pause-key.cjs");

const MAIN_ACTIVITY = `package media.loomarr.tv
import android.os.Build
import android.os.Bundle

import com.facebook.react.ReactActivity

class MainActivity : ReactActivity() {
  override fun getMainComponentName(): String = "main"
}
`;

test("consumes Play/Pause after React Native has seen it", () => {
  const source = consumePlayPauseKey(MAIN_ACTIVITY);

  assert.match(source, /^import android\.view\.KeyEvent$/m);
  assert.match(source, /val handled = super\.dispatchKeyEvent\(event\)/);
  assert.match(source, /event\.keyCode == KeyEvent\.KEYCODE_MEDIA_PLAY_PAUSE \|\| handled/);
});

test("applies once, however often prebuild runs", () => {
  const once = consumePlayPauseKey(MAIN_ACTIVITY);

  assert.equal(consumePlayPauseKey(once), once);
});

test("fails closed when Expo's MainActivity shape changes", () => {
  assert.throws(() => consumePlayPauseKey("class Other {}\n"), /MainActivity/);
});

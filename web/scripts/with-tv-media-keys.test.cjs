const assert = require("node:assert/strict");
const test = require("node:test");
const { keepPlayKeysInTheApp } = require("./with-tv-media-keys.cjs");

const ACTIVITY = `package media.loomarr.tv.prototype
import expo.modules.splashscreen.SplashScreenManager

import android.os.Build

import com.facebook.react.ReactActivity

class MainActivity : ReactActivity() {
  override fun getMainComponentName(): String = "main"

  override fun invokeDefaultOnBackPressed() {
      super.invokeDefaultOnBackPressed()
  }
}
`;

test("the activity reports the play keys as handled, after React has seen them", () => {
  const out = keepPlayKeysInTheApp(ACTIVITY);

  assert.match(out, /^package media\.loomarr\.tv\.prototype\nimport android\.view\.KeyEvent\n/);
  for (const key of ["KEYCODE_MEDIA_PLAY_PAUSE", "KEYCODE_MEDIA_PLAY", "KEYCODE_MEDIA_PAUSE"]) {
    assert.ok(out.includes(`KeyEvent.${key}`), key);
  }
  assert.match(out, /override fun onKeyDown\(keyCode: Int, event: KeyEvent\): Boolean =\s+super\.onKeyDown\(keyCode, event\) \|\| isPlayKey\(keyCode\)/);
  assert.match(out, /override fun onKeyUp\(keyCode: Int, event: KeyEvent\): Boolean =\s+super\.onKeyUp\(keyCode, event\) \|\| isPlayKey\(keyCode\)/);
  // Inside the class: the class's own members are kept and the file still ends with its brace.
  assert.ok(out.indexOf("invokeDefaultOnBackPressed") < out.indexOf("isPlayKey"));
  assert.match(out, /\}\n$/);
});

test("applying it twice changes nothing", () => {
  const once = keepPlayKeysInTheApp(ACTIVITY);
  assert.equal(keepPlayKeysInTheApp(once), once);
});

test("an existing KeyEvent import is not repeated", () => {
  const out = keepPlayKeysInTheApp(ACTIVITY.replace("import android.os.Build", "import android.view.KeyEvent"));
  assert.equal(out.match(/import android\.view\.KeyEvent/g).length, 1);
});

test("fails closed when Expo's activity shape changes", () => {
  assert.throws(() => keepPlayKeysInTheApp("package x\nclass Other {}\n"), /MainActivity's class body/);
});

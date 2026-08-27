const assert = require("node:assert/strict");
const test = require("node:test");
const { addLoomarrReleaseSigning } = require("./with-loomarr-android-release.cjs");

const generatedBuild = `android {
    signingConfigs {
        debug {
            storeFile file('debug.keystore')
            storePassword 'android'
            keyAlias 'androiddebugkey'
            keyPassword 'android'
        }
    }
    buildTypes {
        debug {
            signingConfig signingConfigs.debug
        }
        release {
            signingConfig signingConfigs.debug
        }
    }
}
`;

test("signs only the explicit React Native production release with Loomarr's upload key", () => {
  const generated = addLoomarrReleaseSigning(generatedBuild);

  assert.match(generated, /LOOMARR_ANDROID_RENDERER.*react-native/);
  assert.match(generated, /LOOMARR_ANDROID_KEYSTORE_PATH/);
  assert.match(generated, /LOOMARR_ANDROID_KEYSTORE_PASSWORD/);
  assert.match(generated, /LOOMARR_ANDROID_KEY_ALIAS/);
  assert.match(generated, /LOOMARR_ANDROID_KEY_PASSWORD/);
  assert.match(generated, /throw new GradleException/);
  assert.match(
    generated,
    /signingConfig loomarrReactNativeRelease \? signingConfigs\.release : signingConfigs\.debug/,
  );
  assert.equal(addLoomarrReleaseSigning(generated), generated);
});

test("fails closed when Expo's generated signing shape changes", () => {
  assert.throws(
    () => addLoomarrReleaseSigning("android {\n}\n"),
    /generated Android signing configuration/,
  );
});

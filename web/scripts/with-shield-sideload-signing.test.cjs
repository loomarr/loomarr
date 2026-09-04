const assert = require("node:assert/strict");
const test = require("node:test");
const { addShieldSideloadSigning } = require("./with-shield-sideload-signing.cjs");

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

test("signs only the explicit permanent-identity Shield sideload", () => {
  const generated = addShieldSideloadSigning(generatedBuild);

  assert.match(generated, /LOOMARR_SHIELD_SIDELOAD.*== "1"/);
  assert.match(generated, /LOOMARR_ANDROID_KEYSTORE_PATH/);
  assert.match(generated, /LOOMARR_ANDROID_KEYSTORE_PASSWORD/);
  assert.match(generated, /LOOMARR_ANDROID_KEY_ALIAS/);
  assert.match(generated, /LOOMARR_ANDROID_KEY_PASSWORD/);
  assert.match(generated, /throw new GradleException/);
  assert.match(
    generated,
    /signingConfig loomarrShieldSideload \? signingConfigs\.release : signingConfigs\.debug/,
  );
  assert.equal(addShieldSideloadSigning(generated), generated);
});

test("fails closed when Expo's generated signing shape changes", () => {
  assert.throws(
    () => addShieldSideloadSigning("android {\n}\n"),
    /generated Android signing configuration/,
  );
});

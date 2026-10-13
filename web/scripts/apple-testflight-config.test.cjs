const assert = require("node:assert/strict");
const test = require("node:test");
const { appleTestFlightConfig, appleVersionFields } = require("./apple-testflight-config.cjs");

const release = { label: "Apple TV", name: "Loomarr TV", slug: "loomarr-tv", ios: { bundleIdentifier: "media.loomarr.tv" } };
const development = {
  name: "Loomarr TV Prototype",
  ios: { bundleIdentifier: "media.loomarr.tv.prototype" },
  android: { package: "media.loomarr.tv.prototype" },
};

test("maps release versions onto Apple's version grammar", () => {
  assert.deepEqual(appleVersionFields("iPhone", "v0.2.0", "12"), { version: "0.2.0", buildNumber: "12" });
  assert.deepEqual(appleVersionFields("iPhone", "1.10.3-rc.2", "300"), { version: "1.10.3", buildNumber: "300" });
  // CFBundleShortVersionString is exactly three integers; the build number is a single positive
  // integer without leading zeros.
  assert.throws(() => appleVersionFields("iPhone", "0.2.0.1", "1"), /iPhone release version must be x\.y\.z/);
  assert.throws(() => appleVersionFields("Apple TV", "0.2.0", "007"), /Apple TV release build number must be a positive integer/);
  assert.throws(() => appleVersionFields("Apple TV", "0.2.0", "1.2"), /positive integer/);
});

test("leaves the development identity alone without a channel", () => {
  assert.equal(appleTestFlightConfig(development, release, {}), development);
  assert.throws(
    () => appleTestFlightConfig(development, release, { LOOMARR_APPLE_BUILD_NUMBER: "3" }),
    /Apple TV release version metadata requires LOOMARR_APPLE_RELEASE_CHANNEL/,
  );
});

test("applies only the Apple identity and the release compliance settings", () => {
  const config = appleTestFlightConfig(development, release, {
    LOOMARR_APPLE_RELEASE_CHANNEL: "testflight",
    LOOMARR_APPLE_VERSION: "0.2.0-beta.9",
    LOOMARR_APPLE_BUILD_NUMBER: "7",
  });

  assert.equal(config.name, "Loomarr TV");
  assert.equal(config.slug, "loomarr-tv");
  assert.equal(config.version, "0.2.0");
  assert.equal(config.label, undefined);
  assert.equal(config.scheme, undefined);
  assert.equal(config.ios.bundleIdentifier, "media.loomarr.tv");
  assert.equal(config.ios.buildNumber, "7");
  assert.equal(config.ios.config.usesNonExemptEncryption, false);
  assert.deepEqual(config.android, development.android);
  assert.throws(
    () =>
      appleTestFlightConfig(development, release, {
        LOOMARR_APPLE_RELEASE_CHANNEL: "appstore",
        LOOMARR_APPLE_VERSION: "0.2.0",
        LOOMARR_APPLE_BUILD_NUMBER: "7",
      }),
    /Apple TV release channel must be testflight/,
  );
});

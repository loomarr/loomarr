const { appleTestFlightConfig } = require("../../scripts/apple-testflight-config.cjs");

// The permanent iPhone identity (ADR 0042). The Local Network keys (NSBonjourServices,
// NSLocalNetworkUsageDescription) live in app.json: every build browses for a server, so
// development builds need the same prompt.
const iphoneRelease = {
  label: "iPhone",
  name: "Loomarr",
  slug: "loomarr-mobile",
  scheme: "loomarr",
  ios: {
    bundleIdentifier: "media.loomarr.mobile",
    // App Store Connect lets a record add iPad support later but never remove it.
    supportsTablet: false,
  },
};

module.exports = ({ config }) => appleTestFlightConfig(config, iphoneRelease);

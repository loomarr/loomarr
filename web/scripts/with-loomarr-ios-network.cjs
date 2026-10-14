const { withInfoPlist } = require("@expo/config-plugins");

// The iOS and tvOS twin of with-loomarr-android-network.cjs: a viewer may pair with a server over
// plain HTTP, often under a local DNS name such as `loomarr.lan` or `loomarr.home.arpa` that App
// Transport Security doesn't treat as local (maintainer decision, 2026-10-13).
//
// The policy is replaced, not merged: Apple ignores NSAllowsArbitraryLoads whenever
// NSAllowsLocalNetworking (Expo's template default), NSAllowsArbitraryLoadsInWebContent or
// NSAllowsArbitraryLoadsForMedia is also present, so keeping the template's key would quietly go
// on blocking those servers.
function permitConfiguredLoomarrHttp(infoPlist) {
  return { ...infoPlist, NSAppTransportSecurity: { NSAllowsArbitraryLoads: true } };
}

function withLoomarrIosNetwork(config) {
  return withInfoPlist(config, (plistConfig) => {
    plistConfig.modResults = permitConfiguredLoomarrHttp(plistConfig.modResults);
    return plistConfig;
  });
}

module.exports = withLoomarrIosNetwork;
module.exports.permitConfiguredLoomarrHttp = permitConfiguredLoomarrHttp;

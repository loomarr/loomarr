const assert = require("node:assert/strict");
const test = require("node:test");
const { permitConfiguredLoomarrHttp } = require("./with-loomarr-ios-network.cjs");

test("permits a plain-HTTP Loomarr server under any local DNS name", () => {
  // Expo's template default, which would keep loomarr.lan and loomarr.home.arpa blocked.
  const infoPlist = {
    CFBundleName: "Loomarr",
    NSAppTransportSecurity: { NSAllowsArbitraryLoads: false, NSAllowsLocalNetworking: true },
  };

  const result = permitConfiguredLoomarrHttp(infoPlist);

  // Apple ignores NSAllowsArbitraryLoads while NSAllowsLocalNetworking is present.
  assert.deepEqual(result.NSAppTransportSecurity, { NSAllowsArbitraryLoads: true });
  assert.equal(result.CFBundleName, "Loomarr");
});

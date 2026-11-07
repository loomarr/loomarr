import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

// #1162: Metro once measured image assets with image-size, which detected the parser from the
// bytes, so a `.png` could carry ICNS/JXL/HEIF boxes that looped the parser. Loomarr patched
// image-size@1.2.1 until Metro 0.87.1 replaced it with its own parser selected by the declared
// asset type. These contracts keep that retirement honest: each app's resolved Metro must reject
// the same malformed payloads, bounded, and must not bring image-size back.

// Resolve through each app's real Expo/Metro graph, never an unrelated root
// dependency or a hard-coded pnpm-store path.
const metroAssets = (appName) => {
  const app = createRequire(new URL(`../apps/${appName}/package.json`, import.meta.url));
  const expo = createRequire(app.resolve("expo/metro-config"));
  const config = createRequire(expo.resolve("@expo/metro-config"));
  const metro = createRequire(config.resolve("metro/package.json"));
  return metro.resolve("metro/private/Assets");
};

const runParser = (assets, expression) => {
  const code = `
    const assert = require('node:assert/strict');
    const {getAssetSize} = require(${JSON.stringify(assets)});
    ${expression}
  `;
  // A looping parser must fail this test without hanging or exhausting the
  // test runner. The parent bounds CPU time and each child bounds its JS heap.
  const result = spawnSync(process.execPath, ["--max-old-space-size=64", "-e", code], {
    encoding: "utf8",
    timeout: 3000,
    killSignal: "SIGKILL",
    maxBuffer: 16 * 1024,
  });
  assert.equal(result.error, undefined, `parser child error: ${result.error}`);
  assert.equal(result.signal, null, `parser child terminated: ${result.signal}`);
  assert.equal(result.status, 0, result.stderr);
};

for (const appName of ["tv", "mobile"]) {
  const assets = metroAssets(appName);

  describe(`${appName} Metro asset size`, () => {
    it("does not resolve image-size from Metro", () => {
      // If Metro depends on image-size again, the #1162 parser review comes back with it.
      assert.throws(() => createRequire(assets)("image-size"), { code: "MODULE_NOT_FOUND" });
    });

    it("still reads the installed application's normal PNG asset", () => {
      const png = fileURLToPath(new URL(`../apps/${appName}/assets/icon.png`, import.meta.url));
      runParser(
        assets,
        `
        const data = require('node:fs').readFileSync(${JSON.stringify(png)});
        const size = getAssetSize('png', data, 'icon.png');
        assert.ok(size.width > 0 && size.height > 0);
      `,
      );
    });

    for (const [name, hex] of Object.entries({
      "ICNS zero-length entry": "69636e73000000106963703400000000",
      "ICNS undersized entry": "69636e73000000106963703400000004",
      "ICNS truncated header": "69636e730000001069637034",
      "JXL zero-length partial codestream box":
        "0000000c4a584c200d0a870a0000000c667479706a786c20000000006a786c7000000000",
      "HEIF zero-length box without dimensions": "000000006674797068656963",
    })) {
      it(`rejects ${name} presented as PNG`, () => {
        runParser(
          assets,
          `
          const data = Buffer.from(${JSON.stringify(hex)}, 'hex');
          assert.throws(() => getAssetSize('png', data, 'disguised.png'));
        `,
        );
      });
    }
  });
}

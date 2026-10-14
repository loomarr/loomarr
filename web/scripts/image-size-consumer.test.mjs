import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

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
    const imageSize = require('node:module').createRequire(${JSON.stringify(assets)})('image-size');
    ${expression}
  `;
  // An unpatched parser must fail this test without hanging or exhausting the
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

const consumers = [
  ...["tv", "mobile"].map((appName) => ({ name: appName, appName, assets: metroAssets(appName) })),
  // Exercise the exact locked vulnerable Metro consumer as well as each app's
  // resolved config. A changed Metro pin requires reviewing this regression.
  {
    name: "locked Metro 0.87.0",
    appName: "tv",
    assets: fileURLToPath(
      new URL("../node_modules/.pnpm/metro@0.87.0/node_modules/metro/src/Assets.js", import.meta.url),
    ),
  },
];
for (const { name, appName, assets } of consumers) {
  describe(`${name} image-size consumer`, () => {
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
      it(`rejects ${name} even when presented as PNG`, () => {
        runParser(
          assets,
          `
          const data = Buffer.from(${JSON.stringify(hex)}, 'hex');
          assert.throws(() => imageSize(data));
          assert.throws(() => getAssetSize('png', data, 'disguised.png'));
        `,
        );
      });
    }

    it("preserves a valid ICNS entry and a supported terminal size-zero HEIF box", () => {
      runParser(
        assets,
        `
        const icns = Buffer.from('69636e73000000106963703400000008', 'hex');
        const iconSize = imageSize(icns);
        assert.equal(iconSize.width,16); assert.equal(iconSize.height,16);
        const box = (name, data) => { const header=Buffer.alloc(8); header.writeUInt32BE(data.length+8); header.write(name,4); return Buffer.concat([header,data]); };
        const dimensions=Buffer.alloc(12); dimensions.writeUInt32BE(32,4); dimensions.writeUInt32BE(16,8);
        const meta=box('meta',Buffer.concat([Buffer.alloc(4),box('iprp',box('ipco',box('ispe',dimensions)))]));
        meta.writeUInt32BE(0); // legal final box: extends through remaining input
        const heif=Buffer.concat([box('ftyp',Buffer.from('heic')),meta]);
        const heifSize=imageSize(heif);
        assert.equal(heifSize.width,32); assert.equal(heifSize.height,16);
      `,
      );
    });
  });
}

import { resolve } from "node:path";
import { runInNewContext } from "node:vm";
import { build, loadConfigFromFile } from "vite";
import { describe, expect, it } from "vitest";

// react-native-web's vendored Animated code calls Node's bare `global` (e.g.
// `global.cancelAnimationFrame` when an animation stops). Browsers have no `global`, so a
// hard load that stopped an animation threw "global is not defined" (#2725). The build maps
// `global` onto `globalThis`; this bundles the real vendored module with the app's own resolve and
// define settings and runs it in a context that, like a browser, has no `global`.
const configFile = resolve(process.cwd(), "vite.config.ts");
const animation = resolve(
  process.cwd(),
  "node_modules/react-native-web/dist/vendor/react-native/Animated/animations/TimingAnimation.js",
);

async function bundleTimingAnimation(): Promise<string> {
  const loaded = await loadConfigFromFile({ command: "build", mode: "production" }, configFile);
  if (!loaded) throw new Error("vite.config.ts did not load");
  const result = await build({
    configFile: false,
    logLevel: "silent",
    resolve: loaded.config.resolve,
    // Library mode leaves `process.env.NODE_ENV` alone; the app build replaces it.
    define: { "process.env.NODE_ENV": '"production"', ...loaded.config.define },
    build: {
      write: false,
      minify: false,
      lib: { entry: animation, formats: ["iife"], name: "TimingAnimation" },
    },
  });
  const outputs = Array.isArray(result)
    ? result.flatMap((r) => ("output" in r ? r.output : []))
    : "output" in result
      ? result.output
      : [];
  const chunk = outputs.find((o) => o.type === "chunk");
  if (chunk?.type !== "chunk") throw new Error("no bundle emitted");
  return chunk.code;
}

describe("production bundle", () => {
  it("stops an animation in a browser context that has no `global`", async () => {
    const code = await bundleTimingAnimation();
    const cancelled: number[] = [];
    const browser: Record<string, unknown> = {
      cancelAnimationFrame: (id: number) => cancelled.push(id),
      clearTimeout,
      console,
    };
    browser.window = browser;
    browser.self = browser;
    browser.globalThis = browser;
    const exported = runInNewContext(`${code}\n;TimingAnimation`, browser) as {
      default: { prototype: { stop(this: unknown): void } };
    };
    const Timing = exported.default ?? exported;
    expect(() =>
      Timing.prototype.stop.call({
        __active: true,
        _animationFrame: 7,
        __debouncedOnEnd: () => undefined,
      }),
    ).not.toThrow();
    expect(cancelled).toEqual([7]);
  }, 60_000);
});

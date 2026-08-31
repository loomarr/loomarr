#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

const filesBelow = (root) =>
  readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const file = path.join(root, entry.name);
    return entry.isDirectory() ? filesBelow(file) : [file];
  });

const bundleBytes = (root) => {
  const bundles = filesBelow(root).filter((file) => /\.(hbc|js)$/.test(file));
  if (bundles.length === 0) throw new Error(`Expo export produced no JavaScript bundle in ${root}`);
  return bundles.reduce((total, file) => total + statSync(file).size, 0);
};

const compareBundleBytes = (app, runtimeBytes, compilerBytes) => ({
  app,
  compilerBytes,
  deltaBytes: compilerBytes - runtimeBytes,
  deltaPercent: Number((((compilerBytes - runtimeBytes) / runtimeBytes) * 100).toFixed(3)),
  runtimeBytes,
});

const exportBundle = (app, compilerEnabled, output) => {
  const result = spawnSync(
    "pnpm",
    [
      "--filter",
      `@loomarr/${app}`,
      "exec",
      "expo",
      "export",
      "--platform",
      "android",
      "--output-dir",
      output,
      "--clear",
      "--max-workers",
      "1",
    ],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        CI: "true",
        EXPO_TV: app === "tv" ? "1" : "0",
        LOOMARR_TAMAGUI_COMPILER: compilerEnabled ? "1" : "0",
      },
      stdio: "inherit",
    },
  );
  if (result.status !== 0)
    throw new Error(`${app} ${compilerEnabled ? "compiler" : "runtime"} export failed`);
  return bundleBytes(output);
};

const main = (appNames) => {
  const apps = appNames.length === 0 ? ["mobile", "tv"] : appNames;
  if (apps.some((app) => app !== "mobile" && app !== "tv")) {
    console.error("usage: measure-tamagui-compiler.mjs [mobile] [tv]");
    return 2;
  }
  const root = mkdtempSync(path.join(tmpdir(), "loomarr-tamagui-compiler-"));
  try {
    const results = apps.map((app) => {
      const runtimeBytes = exportBundle(app, false, path.join(root, app, "runtime"));
      const compilerBytes = exportBundle(app, true, path.join(root, app, "compiler"));
      return compareBundleBytes(app, runtimeBytes, compilerBytes);
    });
    console.log(JSON.stringify({ results }, null, 2));
    return 0;
  } finally {
    rmSync(root, { force: true, recursive: true });
  }
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exitCode = main(process.argv.slice(2));
}

export { bundleBytes, compareBundleBytes };

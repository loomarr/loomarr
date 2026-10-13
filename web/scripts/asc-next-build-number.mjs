#!/usr/bin/env node
// The next build number of an Apple app: the caller's floor (CI's run number), or one past the highest build
// App Store Connect already holds for the app, whichever is higher (#1900 item 1).
//
//   node asc-next-build-number.mjs <bundle-id> <floor>
//
// App Store Connect refuses a build number it has seen, or one lower than the latest, so the run
// number alone breaks the first time a build is uploaded from anywhere else (build 5 came from a Mac,
// 2026-10-13). Reads LOOMARR_ASC_KEY_PATH, LOOMARR_ASC_KEY_ID and LOOMARR_ASC_ISSUER_ID, prints only
// the number, and never prints the key or the token.
import { createPrivateKey, sign } from "node:crypto";
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const api = "https://api.appstoreconnect.apple.com";

/** The higher of `floor` and one past the largest whole-number build version. */
const nextBuildNumber = (floor, versions) => {
  const highest = versions.reduce((max, version) => {
    if (!/^[1-9][0-9]*$/.test(version)) return max;
    return Math.max(max, Number(version));
  }, 0);
  return Math.max(floor, highest + 1);
};

const base64url = (value) => Buffer.from(value).toString("base64url");

// ES256 per App Store Connect: a JOSE signature is the raw r||s pair, not DER. A runner's clock can
// run ahead of Apple's (one was 90 s fast), which rejects a token issued in its future, so `iat` is
// back-dated two minutes.
const token = ({ issuerId, keyId, keyPath }) => {
  const now = Math.floor(Date.now() / 1000) - 120;
  const header = base64url(JSON.stringify({ alg: "ES256", kid: keyId, typ: "JWT" }));
  const claims = base64url(
    JSON.stringify({ aud: "appstoreconnect-v1", exp: now + 900, iat: now, iss: issuerId }),
  );
  const key = createPrivateKey(readFileSync(keyPath));
  const signature = sign("sha256", Buffer.from(`${header}.${claims}`), { dsaEncoding: "ieee-p1363", key });
  return `${header}.${claims}.${signature.toString("base64url")}`;
};

const getJson = async (url, bearer) => {
  const response = await fetch(url, { headers: { Authorization: `Bearer ${bearer}` } });
  if (!response.ok)
    throw new Error(`App Store Connect answered ${response.status} for ${new URL(url).pathname}`);
  return response.json();
};

const uploadedVersions = async (bundleId, bearer) => {
  const apps = await getJson(
    `${api}/v1/apps?filter[bundleId]=${encodeURIComponent(bundleId)}&fields[apps]=bundleId`,
    bearer,
  );
  const app = apps.data?.find((candidate) => candidate.attributes?.bundleId === bundleId);
  if (!app) throw new Error(`App Store Connect has no app record for ${bundleId}`);
  const versions = [];
  let next = `${api}/v1/builds?filter[app]=${app.id}&fields[builds]=version&limit=200`;
  while (next) {
    const page = await getJson(next, bearer);
    for (const build of page.data ?? []) versions.push(String(build.attributes?.version ?? ""));
    next = page.links?.next;
  }
  return versions;
};

const main = async () => {
  const [bundleId, floorArgument] = process.argv.slice(2);
  const floor = Number(floorArgument);
  if (!bundleId || !Number.isSafeInteger(floor) || floor < 1) {
    throw new Error("usage: asc-next-build-number.mjs <bundle-id> <positive floor>");
  }
  const credentials = {
    // biome-ignore lint/suspicious/noUndeclaredEnvVars: The uncached TestFlight release reads its credentials outside Turbo tasks.
    issuerId: process.env.LOOMARR_ASC_ISSUER_ID,
    // biome-ignore lint/suspicious/noUndeclaredEnvVars: The uncached TestFlight release reads its credentials outside Turbo tasks.
    keyId: process.env.LOOMARR_ASC_KEY_ID,
    // biome-ignore lint/suspicious/noUndeclaredEnvVars: The uncached TestFlight release reads its credentials outside Turbo tasks.
    keyPath: process.env.LOOMARR_ASC_KEY_PATH,
  };
  if (!credentials.issuerId || !credentials.keyId || !credentials.keyPath) {
    throw new Error("LOOMARR_ASC_KEY_PATH, LOOMARR_ASC_KEY_ID and LOOMARR_ASC_ISSUER_ID are required");
  }
  const versions = await uploadedVersions(bundleId, token(credentials));
  process.stdout.write(`${nextBuildNumber(floor, versions)}\n`);
};

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
  main().catch((error) => {
    process.stderr.write(`asc-next-build-number: ${error.message}\n`);
    process.exit(1);
  });
}

export { nextBuildNumber };

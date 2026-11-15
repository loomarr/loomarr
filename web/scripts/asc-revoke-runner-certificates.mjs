#!/usr/bin/env node
// Revoke the Apple Development certificates a TestFlight runner created, so they don't pile up.
//
//   security find-certificate -a -p -c "Apple Development" | node asc-revoke-runner-certificates.mjs
//
// The archive signs automatically with the team's API key. A hosted Mac starts with an empty keychain,
// so every run makes Xcode create a new Development certificate, and nothing ever revoked one: the team
// reached Apple's limit and every upload failed at the archive (2026-11-13, and again 2026-11-15).
// The export signs for the App Store with Apple's cloud-managed distribution certificate, which never
// reaches the keychain, so the runner's Development certificate is spent once the export is done.
//
// It revokes a certificate only when all three hold: its serial is one of the certificates in this
// runner's keychain (read from stdin as PEM); App Store Connect lists it as a Development certificate;
// and it was issued in the last week (it expires more than 358 days from now). A Mac's own certificate
// is never in a runner's keychain, and the last check keeps an older one safe even if it were.
// Distribution certificates are never touched. Reads the same LOOMARR_ASC_* variables as
// asc-next-build-number.mjs and never prints the key or the token.
import { X509Certificate } from "node:crypto";
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

import { describeRefusal, token } from "./asc-next-build-number.mjs";

const api = "https://api.appstoreconnect.apple.com";

const DEVELOPMENT_TYPES = new Set(["DEVELOPMENT", "IOS_DEVELOPMENT"]);

/** A certificate issued within the last week still has more than this left of its year. */
const FRESH_MS = 358 * 24 * 60 * 60 * 1000;

/** Serials compare as hex without leading zeros, whatever the case. */
const normalSerial = (serial) => String(serial).toUpperCase().replace(/^0+/, "");

/** The serials of every certificate in a PEM bundle. */
const pemSerials = (bundle) =>
  (bundle.match(/-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/g) ?? []).map((pem) =>
    normalSerial(new X509Certificate(pem).serialNumber),
  );

/** The App Store Connect certificate records this runner may revoke. */
const revocable = (records, serials, nowMs) => {
  const wanted = new Set(serials.map(normalSerial));
  return records.filter(({ attributes = {} }) => {
    const expires = Date.parse(attributes.expirationDate ?? "");
    return (
      wanted.has(normalSerial(attributes.serialNumber ?? "")) &&
      DEVELOPMENT_TYPES.has(attributes.certificateType) &&
      Number.isFinite(expires) &&
      expires - nowMs > FRESH_MS
    );
  });
};

const call = async (method, path, bearer) => {
  const response = await fetch(`${api}${path}`, { headers: { Authorization: `Bearer ${bearer}` }, method });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new Error(describeRefusal(response.status, path.split("?")[0], body));
  }
  return method === "GET" ? response.json() : null;
};

const main = async () => {
  const serials = pemSerials(readFileSync(0, "utf8"));
  if (serials.length === 0) {
    process.stdout.write(
      "asc-revoke-runner-certificates: no Development certificate in this runner's keychain\n",
    );
    return;
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
  const bearer = token(credentials);
  const fields = "fields[certificates]=serialNumber,certificateType,expirationDate";
  const records = [];
  for (const serial of serials) {
    const page = await call("GET", `/v1/certificates?filter[serialNumber]=${serial}&${fields}`, bearer);
    records.push(...(page.data ?? []));
  }
  for (const record of revocable(records, serials, Date.now())) {
    await call("DELETE", `/v1/certificates/${encodeURIComponent(record.id)}`, bearer);
    process.stdout.write(
      `asc-revoke-runner-certificates: revoked this runner's Development certificate ${record.attributes.serialNumber}\n`,
    );
  }
};

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
  main().catch((error) => {
    process.stderr.write(`asc-revoke-runner-certificates: ${error.message}\n`);
    process.exit(1);
  });
}

export { normalSerial, pemSerials, revocable };

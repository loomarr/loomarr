#!/usr/bin/env node
// Validates the app-level PrivacyInfo.xcprivacy of a TestFlight IPA, given as JSON (plutil -convert
// json) in argv[2]. React Native's pod install aggregates every statically linked pod's
// required-reason API declarations into the app manifest, so the entry count is not ours to pin.
// What matters: no tracking, our own UserDefaults/CA92.1 declaration survives, and every entry is a
// complete, unique declaration. On success prints the declared categories as JSON for the evidence.
"use strict";

const USER_DEFAULTS = "NSPrivacyAccessedAPICategoryUserDefaults";
const USER_DEFAULTS_REASON = "CA92.1";

const fail = (problem) => {
  console.error("ios-testflight: IPA privacy manifest " + problem);
  process.exit(1);
};

const nonEmptyString = (value) => typeof value === "string" && value !== "";

let manifest;
try {
  manifest = JSON.parse(process.argv[2] ?? "");
} catch {
  fail("is not valid JSON");
}
if (manifest === null || typeof manifest !== "object") {
  fail("is not a dictionary");
}

if (manifest.NSPrivacyTracking !== undefined && manifest.NSPrivacyTracking !== false) {
  fail("declares NSPrivacyTracking");
}
const domains = manifest.NSPrivacyTrackingDomains;
if (domains !== undefined && (!Array.isArray(domains) || domains.length > 0)) {
  fail("declares NSPrivacyTrackingDomains");
}

const types = manifest.NSPrivacyAccessedAPITypes;
if (!Array.isArray(types) || types.length === 0) {
  fail("has no accessed-API entries");
}

const categories = [];
const seen = new Set();
for (const entry of types) {
  const type = entry?.NSPrivacyAccessedAPIType;
  const reasons = entry?.NSPrivacyAccessedAPITypeReasons;
  if (!nonEmptyString(type)) {
    fail("has an accessed-API entry with no NSPrivacyAccessedAPIType");
  }
  if (!Array.isArray(reasons) || reasons.length === 0 || !reasons.every(nonEmptyString)) {
    fail(type + " has no reasons");
  }
  if (seen.has(type)) {
    fail("declares " + type + " more than once");
  }
  seen.add(type);
  categories.push({ type, reasons });
}

const userDefaults = categories.find((category) => category.type === USER_DEFAULTS);
if (userDefaults === undefined || !userDefaults.reasons.includes(USER_DEFAULTS_REASON)) {
  fail("does not declare " + USER_DEFAULTS + " with reason " + USER_DEFAULTS_REASON);
}

process.stdout.write(JSON.stringify(categories) + "\n");

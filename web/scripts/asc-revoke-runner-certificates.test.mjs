import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { normalSerial, pemSerials, revocable } from "./asc-revoke-runner-certificates.mjs";

// A throwaway self-signed certificate (serial 0A1B2C3D4E5F) standing in for a runner's keychain export.
const FIXTURE_PEM = `-----BEGIN CERTIFICATE-----
MIIBkDCCATWgAwIBAgIGChssPU5fMAoGCCqGSM49BAMCMCQxIjAgBgNVBAMMGVRl
c3QgRGV2ZWxvcG1lbnQ6IEZpeHR1cmUwHhcNMjYxMTE1MDAzNzU1WhcNMjcxMTE1
MDAzNzU1WjAkMSIwIAYDVQQDDBlUZXN0IERldmVsb3BtZW50OiBGaXh0dXJlMFkw
EwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEOmE1Xwq/VtKdbBZgdHvZcFCP0/1ZjCLR
0WU1E3BuudEJrmUAZMurYzG2AVLZn4KjvlQCQhPOAI8VHvERh/0mHKNTMFEwHQYD
VR0OBBYEFLkH00eSr+tOWu88VPeAGzmMaWW2MB8GA1UdIwQYMBaAFLkH00eSr+tO
Wu88VPeAGzmMaWW2MA8GA1UdEwEB/wQFMAMBAf8wCgYIKoZIzj0EAwIDSQAwRgIh
AMbnphEWoQOnTLuTBg20QgelmbQ50TxIau3Qv2X7WOQlAiEA5yCfcq7qE0eiebJB
2yzRag0YUKV1z/kfA6vI/aDVe8k=
-----END CERTIFICATE-----`;

const NOW = Date.parse("2026-11-15T00:00:00Z");
const record = (id, attributes) => ({ attributes, id, type: "certificates" });
const fresh = { certificateType: "DEVELOPMENT", expirationDate: "2027-11-14T23:00:00.000+0000" };

describe("pemSerials", () => {
  it("reads every certificate's serial from a keychain export", () => {
    assert.deepEqual(pemSerials(`${FIXTURE_PEM}\n${FIXTURE_PEM}\n`), ["A1B2C3D4E5F", "A1B2C3D4E5F"]);
  });

  it("finds none in an empty keychain", () => {
    assert.deepEqual(pemSerials(""), []);
  });
});

describe("normalSerial", () => {
  it("ignores case and leading zeros", () => {
    assert.equal(normalSerial("0a1b2c"), normalSerial("A1B2C"));
  });
});

describe("revocable", () => {
  it("revokes the runner's own fresh Development certificate", () => {
    const mine = record("1", { ...fresh, serialNumber: "0A1B2C3D4E5F" });
    assert.deepEqual(revocable([mine], ["A1B2C3D4E5F"], NOW), [mine]);
  });

  it("keeps a certificate that is not in the runner's keychain", () => {
    const other = record("2", { ...fresh, serialNumber: "FFFF" });
    assert.deepEqual(revocable([other], ["A1B2C3D4E5F"], NOW), []);
  });

  it("never revokes a Distribution certificate, even one in the keychain", () => {
    const distribution = record("3", {
      ...fresh,
      certificateType: "DISTRIBUTION",
      serialNumber: "A1B2C3D4E5F",
    });
    assert.deepEqual(revocable([distribution], ["A1B2C3D4E5F"], NOW), []);
  });

  it("keeps a Development certificate issued more than a week ago, like a Mac's own", () => {
    const mac = record("4", {
      certificateType: "DEVELOPMENT",
      expirationDate: "2027-08-31T00:00:00.000+0000",
      serialNumber: "A1B2C3D4E5F",
    });
    assert.deepEqual(revocable([mac], ["A1B2C3D4E5F"], NOW), []);
  });

  it("keeps a record whose expiry it cannot read", () => {
    const unreadable = record("5", { certificateType: "DEVELOPMENT", serialNumber: "A1B2C3D4E5F" });
    assert.deepEqual(revocable([unreadable], ["A1B2C3D4E5F"], NOW), []);
  });
});

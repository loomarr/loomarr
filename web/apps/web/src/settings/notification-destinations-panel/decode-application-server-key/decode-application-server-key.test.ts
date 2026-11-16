import { describe, expect, it } from "vitest";
import { decodeApplicationServerKey } from "./decode-application-server-key";

describe("decodeApplicationServerKey", () => {
  it("decodes unpadded URL-safe base64 to its bytes", () => {
    // "-_" are "+/" in URL-safe base64: 0xfb 0xff 0xfe is "+//+", written "-__-".
    expect([...decodeApplicationServerKey("-__-")]).toEqual([0xfb, 0xff, 0xfe]);
  });

  it("pads a value whose length is not a multiple of four", () => {
    expect([...decodeApplicationServerKey("TWE")]).toEqual([77, 97]);
    expect([...decodeApplicationServerKey("TQ")]).toEqual([77]);
  });
});

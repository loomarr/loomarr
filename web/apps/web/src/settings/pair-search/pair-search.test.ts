import { describe, expect, it } from "vitest";
import { validatePairSearch } from "./pair-search";

const request = "Zb3k_Q9-xY2mN4pL7vR1sT6uW8aC0dEf";

// `next` is navigated to right after someone approves, so the refusals are the point.
describe("validatePairSearch: next", () => {
  it("keeps the server's own continue step", () => {
    const next = `/v1/oauth/continue?request=${request}`;
    expect(validatePairSearch({ next }).next).toBe(next);
  });

  it("refuses an absolute or protocol-relative address", () => {
    expect(
      validatePairSearch({ next: `https://evil.example/v1/oauth/continue?request=${request}` }).next,
    ).toBeUndefined();
    expect(
      validatePairSearch({ next: `//evil.example/v1/oauth/continue?request=${request}` }).next,
    ).toBeUndefined();
  });

  it("refuses any other path on this server", () => {
    expect(validatePairSearch({ next: "/settings/access" }).next).toBeUndefined();
    expect(
      validatePairSearch({ next: `/v1/oauth/continue/../../logout?request=${request}` }).next,
    ).toBeUndefined();
  });

  it("refuses extra parameters and a backslash", () => {
    expect(
      validatePairSearch({ next: `/v1/oauth/continue?request=${request}&next=//evil` }).next,
    ).toBeUndefined();
    expect(validatePairSearch({ next: `/v1/oauth/continue?request=${request}\\evil` }).next).toBeUndefined();
  });

  it("ignores a non-string", () => {
    expect(validatePairSearch({ next: ["/v1/oauth/continue"] }).next).toBeUndefined();
  });
});

describe("validatePairSearch: code", () => {
  it("keeps only the characters a user code can have", () => {
    expect(validatePairSearch({ code: "bcdf-ghjk<script>" }).code).toBe("BCDF-GHJK");
  });
});

import { describe, expect, it } from "vitest";
import { Route } from "@/routes/_authed/library/index";

// What the Library's URL may carry into the list request: a hand-typed link is narrowed here.
const validate = Route.options.validateSearch as (search: Record<string, unknown>) => Record<string, unknown>;

describe("the Library's URL filters", () => {
  it("keeps the filters, a number among them read as text", () => {
    expect(
      validate({ studio: 1917, person: "p1", audio: "eng", subtitles: "fre", uhd: "true", hdr: "any" }),
    ).toEqual({ studio: "1917", person: "p1", audio: "eng", subtitles: "fre", uhd: true, hdr: "any" });
  });

  // The server lists nothing for a studio or a person it doesn't know, so they pass.
  it("passes an unknown studio or person, cut to what the server takes", () => {
    expect(validate({ studio: "x".repeat(300), person: "y".repeat(900) })).toEqual({
      studio: "x".repeat(200),
      person: "y".repeat(600),
    });
  });

  it.each([
    ["a blank", "  "],
    ["an object", { id: "p1" }],
    ["a list", ["p1"]],
    ["a boolean", true],
  ])("drops a person id that is %s", (_what, person) => {
    expect(validate({ person })).toEqual({});
  });

  it.each([
    [{ uhd: true }, { uhd: true }],
    [{ uhd: false }, {}],
    [{ uhd: "yes" }, {}],
    [{ uhd: 1 }, {}],
    [{ hdr: "Dolby Vision" }, { hdr: "Dolby Vision" }],
    [{ hdr: "HDR11" }, {}],
    [{ hdr: true }, {}],
  ])("reads %o as %o", (search, expected) => {
    expect(validate(search)).toEqual(expected);
  });
});

import { describe, expect, it } from "vitest";
import { validateCatalogSearch } from "./filler-search";

describe("validateCatalogSearch", () => {
  it("keeps supported library filters and normalizes paging", () => {
    expect(
      validateCatalogSearch({
        q: "cereal",
        role: "commercial",
        audience: "kids",
        taxon: "breakfast-food",
        withoutAxis: "seasonal",
        untagged: "true",
        view: "list",
        page: "3.9",
        parent: "reel-hash",
      }),
    ).toEqual({
      q: "cereal",
      role: "commercial",
      audience: "kids",
      taxon: "breakfast-food",
      withoutAxis: "seasonal",
      untagged: true,
      view: "list",
      page: 3,
      parent: "reel-hash",
    });
  });

  it("drops invalid or default values before they reach the API", () => {
    expect(
      validateCatalogSearch({
        role: "programme",
        audience: "adults-only",
        taxon: "not valid!",
        withoutAxis: "era",
        view: "grid",
        page: 0,
        parent: "x".repeat(129),
      }),
    ).toEqual({});
  });
});

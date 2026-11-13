import { describe, expect, it } from "vitest";
import { CATALOG_PAGE_SIZE, catalogListParams, validateCatalogSearch } from "./filler-search";

describe("catalogListParams", () => {
  it("opens the unfiltered catalog on top-level clips and composites, one page", () => {
    expect(catalogListParams({})).toEqual({
      includeComposites: true,
      topLevel: true,
      limit: CATALOG_PAGE_SIZE,
    });
  });

  it("drops the top-level restriction once a filter narrows the list, and pages by offset", () => {
    expect(catalogListParams({ q: "cereal", role: "commercial", page: 3 })).toEqual({
      q: "cereal",
      role: "commercial",
      limit: CATALOG_PAGE_SIZE,
      offset: 2 * CATALOG_PAGE_SIZE,
    });
  });

  it("reads a composite's own clips by parent, whatever else is set", () => {
    expect(catalogListParams({ parent: "reel-hash" })).toEqual({
      parentHash: "reel-hash",
      limit: CATALOG_PAGE_SIZE,
    });
  });
});

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

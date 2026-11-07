import { TaxonDTOAxis } from "@loomarr/api/models/taxonDTOAxis";
import { describe, expect, it } from "vitest";
import { isTaxonomyAxis, TAXONOMY_AXES } from "./taxonomy-axes";

describe("taxonomy axes", () => {
  it("lists every generated axis once, product first", () => {
    expect(TAXONOMY_AXES).toEqual(["product", "format", "presentation", "seasonal", "audience-cue"]);
    expect([...TAXONOMY_AXES].sort()).toEqual(Object.values(TaxonDTOAxis).sort());
  });

  it("recognises only axis slugs", () => {
    expect(isTaxonomyAxis("seasonal")).toBe(true);
    expect(isTaxonomyAxis("audience")).toBe(false);
    expect(isTaxonomyAxis("toString")).toBe(false);
    expect(isTaxonomyAxis(undefined)).toBe(false);
  });
});

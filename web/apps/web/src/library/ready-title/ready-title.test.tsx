import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { readyTitleOf } from "./ready-title";

const PNG =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQzwAEjP8ZAAoDAv8T7QhZAAAAAElFTkSuQmCC";

const poster: ImageDTO = {
  hash: "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503",
  role: "poster",
  width: 500,
  height: 750,
  placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
  dominantHex: "#7a5230",
  animated: false,
  srcSetWebp: `${PNG} 500w`,
  srcSetAvif: `${PNG} 500w`,
  src: PNG,
};

const item = (over: Partial<LibraryItemDTO> = {}): LibraryItemDTO => ({
  id: "90000001",
  kind: "movie",
  name: "The Lighthouse Keeper",
  ready: true,
  files: 1,
  extraCopies: 0,
  unplayable: 0,
  ...over,
});

// "Ready so far" names each title and shows its poster when it has one; a title without one gets
// the plain tile, so nothing is drawn for it here.
describe("readyTitleOf", () => {
  it("carries the title's id and name", () => {
    const title = readyTitleOf(item());
    expect(title.id).toBe("90000001");
    expect(title.name).toBe("The Lighthouse Keeper");
  });

  it("draws the poster at the tile's width, as decoration", () => {
    const { container } = render(<>{readyTitleOf(item({ poster })).poster}</>);
    expect(container.querySelector("img")?.getAttribute("alt")).toBe("");
    expect(container.querySelector("source")?.getAttribute("sizes")).toBe("64px");
  });

  it("leaves the poster out when the title has none", () => {
    expect(readyTitleOf(item()).poster).toBeUndefined();
  });
});

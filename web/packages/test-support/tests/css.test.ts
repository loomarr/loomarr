import { describe, expect, it } from "vitest";

import { stylesOfText, withoutRuntimeMarker } from "../css";

const markup =
  '<style>._a-1{color:red} ._a-10{color:blue} ._b-2:hover{width:1px}</style><span class="x _a-1 _b-2">Hi</span>';

describe("atomic css", () => {
  it("resolves an element's atomic classes to their declarations, without prefix collisions", () => {
    expect(stylesOfText(markup, "Hi")).toBe("color:red;width:1px");
  });

  it("strips only Tamagui's config-revision marker", () => {
    const marker = '<span aria-hidden="true" hidden="" data-tamagui-config-revision="1"></span>';
    expect(withoutRuntimeMarker(`${marker}<p>x</p>`)).toBe("<p>x</p>");
  });
});

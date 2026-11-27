import { describe, expect, it } from "vitest";
import { TAXONOMY_AXES } from "../../taxonomy-axes";
import { AXIS_COPY } from "./taxonomy-axis-copy";

describe("AXIS_COPY", () => {
  it("describes every taxonomy axis with a distinct label, help and example", () => {
    expect(Object.keys(AXIS_COPY).sort()).toEqual([...TAXONOMY_AXES].sort());
    const labels = TAXONOMY_AXES.map((axis) => AXIS_COPY[axis].label);
    expect(new Set(labels).size).toBe(labels.length);
    for (const axis of TAXONOMY_AXES) {
      expect(AXIS_COPY[axis].help).not.toBe("");
      expect(AXIS_COPY[axis].example).toMatch(/^For example: /);
    }
  });
});

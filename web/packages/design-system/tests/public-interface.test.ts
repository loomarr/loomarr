import { describe, expect, it } from "vitest";

import { Body, Heading, LoomarrProvider, Panel, Screen } from "../index";

describe("design-system public interface", () => {
  it("exposes the provider and the first universal primitives", () => {
    expect([LoomarrProvider, Screen, Panel, Heading, Body]).not.toContain(undefined);
  });
});

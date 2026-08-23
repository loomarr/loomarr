import { describe, expect, it } from "vitest";

import { ClientPlatformProof } from "../index";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
  });
});

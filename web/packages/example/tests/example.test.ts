import { describe, expect, it } from "vitest";
import { welcomeViewer } from "../index";

describe("example deep module", () => {
  it("exercises behavior through its public interface", () => {
    expect(welcomeViewer(" Dana ")).toBe("Welcome, Dana.");
  });
});

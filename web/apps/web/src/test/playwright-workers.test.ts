import { describe, expect, it } from "vitest";
import { localWorkerCount } from "../../playwright.shared";

describe("local Playwright worker budget", () => {
  it("falls back to one browser under memory pressure", () => {
    expect(localWorkerCount(0)).toBe(1);
    expect(localWorkerCount(1.49 * 1024 ** 3)).toBe(1);
  });

  it("scales by the memory allowance without exceeding four browsers", () => {
    expect(localWorkerCount(3 * 1024 ** 3)).toBe(2);
    expect(localWorkerCount(20 * 1024 ** 3)).toBe(4);
  });
});

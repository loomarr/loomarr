import { describe, expect, it } from "vitest";
import { intentSchema } from "../schemas";
import { CHANNEL_TEMPLATES } from "./templates";

describe("channel templates", () => {
  it("ships the starter set §13 names", () => {
    expect(CHANNEL_TEMPLATES.length).toBeGreaterThanOrEqual(4);
    expect(new Set(CHANNEL_TEMPLATES.map((t) => t.id)).size).toBe(CHANNEL_TEMPLATES.length);
  });

  // A template exists to prefill the intent form — one that fails its own validation
  // would hand the user a broken starting point.
  it("every template is a valid intent", () => {
    for (const t of CHANNEL_TEMPLATES) {
      const parsed = intentSchema.safeParse(t.intent);
      expect(parsed.success).toBe(true);
    }
  });
});

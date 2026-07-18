import { describe, expect, it } from "vitest";
import { bootstrapSchema, intentSchema, loginSchema } from "./schemas";

describe("intentSchema", () => {
  it("requires a description of at least 3 chars", () => {
    expect(intentSchema.safeParse({ description: "hi" }).success).toBe(false);
    expect(intentSchema.safeParse({ description: "90s action movies" }).success).toBe(true);
  });
});

describe("bootstrapSchema", () => {
  it("rejects short passwords and mismatched confirmations", () => {
    expect(bootstrapSchema.safeParse({ username: "a", password: "short", confirm: "short" }).success).toBe(
      false,
    );
    expect(
      bootstrapSchema.safeParse({ username: "a", password: "longenough", confirm: "different" }).success,
    ).toBe(false);
    expect(
      bootstrapSchema.safeParse({ username: "a", password: "longenough", confirm: "longenough" }).success,
    ).toBe(true);
  });
});

describe("loginSchema", () => {
  it("requires both fields", () => {
    expect(loginSchema.safeParse({ username: "", password: "x" }).success).toBe(false);
    expect(loginSchema.safeParse({ username: "matt", password: "x" }).success).toBe(true);
  });
});

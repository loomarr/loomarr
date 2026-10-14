import { describe, expect, it } from "vitest";
import { bootstrapSchema, intentSchema, loginSchema, notificationProviderFormSchema } from "./schemas";

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

describe("notificationProviderFormSchema", () => {
  const values = {
    label: "Operations Slack",
    events: ["channel_degraded"],
    enabled: true,
    settings: {},
    clearedSecrets: [],
  };

  it("requires an event and each server-defined required setting", () => {
    const schema = notificationProviderFormSchema([{ key: "webhookUrl", label: "Slack webhook URL" }]);
    const result = schema.safeParse({ ...values, events: [] });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ message: "Choose at least one event.", path: ["events"] }),
          expect.objectContaining({ message: "Enter Slack webhook URL.", path: ["settings", "webhookUrl"] }),
        ]),
      );
    }
  });

  it("preserves an omitted configured secret but rejects explicitly clearing a required one", () => {
    const schema = notificationProviderFormSchema(
      [{ key: "webhookUrl", label: "Slack webhook URL" }],
      ["webhookUrl"],
    );

    expect(schema.safeParse(values).success).toBe(true);
    expect(schema.safeParse({ ...values, clearedSecrets: ["webhookUrl"] }).success).toBe(false);
  });
});

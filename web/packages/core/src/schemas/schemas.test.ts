import { describe, expect, it } from "vitest";
import {
  bootstrapSchema,
  changePasswordSchema,
  contactAddressSchema,
  createInvitationSchema,
  createLocalUserSchema,
  devicePairApproveSchema,
  intentSchema,
  loginSchema,
  notificationProviderFormSchema,
  passwordRecoveryRequestSchema,
  passwordRecoveryResetSchema,
  redeemInvitationSchema,
  resetUserPasswordSchema,
} from "./schemas";

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

const messages = (result: {
  success: boolean;
  error?: { issues: { message: string; path: PropertyKey[] }[] };
}) => result.error?.issues.map((issue) => [issue.path.join("."), issue.message]) ?? [];

describe("password forms", () => {
  it("asks for the current password, the minimum, and a matching confirmation", () => {
    expect(messages(changePasswordSchema.safeParse({ current: "", next: "short", confirm: "x" }))).toEqual([
      ["current", "Enter your current password."],
      ["next", "Use at least 8 characters."],
      ["confirm", "New passwords don't match."],
    ]);
    expect(
      messages(changePasswordSchema.safeParse({ current: "old", next: "longenough", confirm: "other" })),
    ).toEqual([["confirm", "New passwords don't match."]]);
  });

  it("trims a new local account's username and takes the wire's roles", () => {
    expect(createLocalUserSchema.parse({ username: " ana ", password: "longenough", role: "admin" })).toEqual(
      {
        username: "ana",
        password: "longenough",
        role: "admin",
      },
    );
    expect(
      messages(createLocalUserSchema.safeParse({ username: " ", password: "longenough", role: "member" })),
    ).toEqual([["username", "Pick a username."]]);
    expect(
      createLocalUserSchema.safeParse({ username: "ana", password: "longenough", role: "owner" }).success,
    ).toBe(false);
  });

  it("checks a Library invitation's username and a local invitation's confirmation", () => {
    const blank = { username: "", password: "longenough", confirm: "" };
    expect(messages(redeemInvitationSchema(true).safeParse(blank))).toEqual([
      ["username", "Enter the username for the invited Library account."],
    ]);
    expect(messages(redeemInvitationSchema(false).safeParse(blank))).toEqual([
      ["confirm", "Passwords need to match."],
    ]);
    expect(redeemInvitationSchema(false).safeParse({ ...blank, confirm: "longenough" }).success).toBe(true);
  });

  it("validates both recovery steps", () => {
    expect(messages(passwordRecoveryRequestSchema.safeParse({ username: "  " }))).toEqual([
      ["username", "Enter your username."],
    ]);
    expect(
      messages(passwordRecoveryResetSchema.safeParse({ password: "longenough", confirm: "nope" })),
    ).toEqual([["confirm", "Passwords need to match."]]);
    expect(messages(resetUserPasswordSchema.safeParse({ next: "short" }))).toEqual([
      ["next", "Use at least 8 characters."],
    ]);
  });

  it("accepts any complete-looking contact email and trims it", () => {
    expect(contactAddressSchema.parse({ email: " ana@example.test " })).toEqual({
      email: "ana@example.test",
    });
    for (const email of ["ana", "@example.test", "ana@"]) {
      expect(messages(contactAddressSchema.safeParse({ email }))).toEqual([
        ["email", "Enter one complete email address."],
      ]);
    }
  });
});

describe("createInvitationSchema", () => {
  const values = {
    kind: "local",
    username: "ana",
    libraryUserId: "",
    contactEmail: "",
    role: "member",
    sendEmail: false,
  };

  it("asks a local invitation for a username and a Library one for an account", () => {
    expect(messages(createInvitationSchema.safeParse({ ...values, username: "  " }))).toEqual([
      ["username", "Choose a username to reserve."],
    ]);
    expect(messages(createInvitationSchema.safeParse({ ...values, kind: "library" }))).toEqual([
      ["libraryUserId", "Choose an Emby or Jellyfin account."],
    ]);
    expect(
      createInvitationSchema.safeParse({ ...values, kind: "library", libraryUserId: "ada" }).success,
    ).toBe(true);
  });

  it("trims the username and the optional contact email, and takes the wire's roles", () => {
    expect(
      createInvitationSchema.parse({
        ...values,
        username: " ana ",
        contactEmail: " a@b.test ",
        role: "admin",
      }),
    ).toEqual({ ...values, username: "ana", contactEmail: "a@b.test", role: "admin" });
    expect(createInvitationSchema.safeParse({ ...values, role: "owner" }).success).toBe(false);
  });

  it("lets the contact email stay blank but not be half an address", () => {
    expect(createInvitationSchema.safeParse(values).success).toBe(true);
    expect(messages(createInvitationSchema.safeParse({ ...values, contactEmail: "ana@" }))).toEqual([
      ["contactEmail", "Enter one complete email address."],
    ]);
  });
});

describe("devicePairApproveSchema", () => {
  it("blocks a blank code and trims a typed one", () => {
    expect(devicePairApproveSchema.safeParse({ userCode: "  " }).success).toBe(false);
    expect(devicePairApproveSchema.parse({ userCode: " BCDF-GHJK " })).toEqual({ userCode: "BCDF-GHJK" });
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

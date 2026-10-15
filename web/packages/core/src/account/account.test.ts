import { describe, expect, it, vi } from "vitest";

import { createAccountSource } from "./account";

describe("account source", () => {
  it("reads the name and role from the generated me route", async () => {
    const request = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ id: "u1", name: "Sam", role: "admin" }), { status: 200 }),
      );
    const signal = new AbortController().signal;

    await expect(createAccountSource(request).load(signal)).resolves.toEqual({ name: "Sam", role: "admin" });
    expect(request).toHaveBeenCalledWith("/v1/auth/me", { method: "GET", signal });
  });

  it("rejects, rather than inventing a name, when the server refuses", async () => {
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 503 }));
    await expect(createAccountSource(request).load(new AbortController().signal)).rejects.toThrow(
      "Couldn't read the account (503).",
    );
  });
});

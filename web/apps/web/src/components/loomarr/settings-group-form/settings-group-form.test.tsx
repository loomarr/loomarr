import type { SettingEntry } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { changedEntries, SettingsGroupForm } from "./settings-group-form";

const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key" | "kind">): SettingEntry => ({
  group: "connections.media_server",
  doc: "help text",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

const entries = [
  entry({ key: "library.url", kind: "url", value: "http://x" }),
  entry({ key: "library.token", kind: "secret", secret: true, preview: "…9f3c", value: "" }),
  entry({
    key: "season.precision",
    kind: "enum",
    enum: ["series", "season"],
    value: "series",
    advanced: true,
  }),
];

const renderForm = (props: Partial<Parameters<typeof SettingsGroupForm>[0]> = {}) =>
  render(<SettingsGroupForm entries={entries} onSave={vi.fn()} {...props} />);

describe("changedEntries", () => {
  it("maps positional values back to their dotted keys, keeping only what changed", () => {
    // Values are positional because a dot in a TanStack Form field name means a nested
    // path — and every registry key is dotted.
    expect(changedEntries(["http://x", "", "series"], entries)).toEqual({});
    expect(changedEntries(["http://y", "", "series"], entries)).toEqual({ "library.url": "http://y" });
  });

  it("omits an untouched secret, which would otherwise CLEAR it", () => {
    // A stored secret reads back as "" (§4 never echoes it). Submitting that "" would be
    // an empty-string PATCH, which clears an optional key (§9) — silently wiping it.
    const submitted = changedEntries(["http://y", "", "series"], entries);
    expect(submitted).not.toHaveProperty("library.token");
  });

  it("includes a secret the operator actually replaced", () => {
    expect(changedEntries(["http://x", "new-token", "series"], entries)).toEqual({
      "library.token": "new-token",
    });
  });
});

describe("SettingsGroupForm", () => {
  it("hides advanced keys until asked, then reveals them", async () => {
    renderForm();
    expect(screen.queryByLabelText("Season precision")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /show advanced \(1\)/i }));
    expect(screen.getByLabelText("Season precision")).toBeInTheDocument();
  });

  it("saves only the fields the operator changed", async () => {
    const onSave = vi.fn();
    renderForm({ onSave });

    await userEvent.type(screen.getByLabelText("Library URL"), "y");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledWith({ "library.url": "http://xy" });
  });

  it("never submits an untouched secret (it would clear the stored value)", async () => {
    const onSave = vi.fn();
    renderForm({ onSave });

    await userEvent.type(screen.getByLabelText("Library URL"), "z");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave.mock.calls[0]?.[0]).not.toHaveProperty("library.token");
  });

  it("submits nothing when the operator changed nothing", async () => {
    const onSave = vi.fn();
    renderForm({ onSave });

    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledWith({});
  });

  it("runs the group's live check and reports a failure as words", async () => {
    const onTest = vi.fn();
    renderForm({ onTest, testOk: false, testHint: "Emby refused the token." });

    await userEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(onTest).toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("Emby refused the token.");
  });

  it("disables the actions while saving", () => {
    renderForm({ onTest: vi.fn(), saving: true });
    expect(screen.getByRole("button", { name: /saving/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /test connection/i })).toBeDisabled();
  });

  it("shows a per-key problem from the last save", () => {
    renderForm({ results: [{ key: "library.url", status: "invalid", problem: "must include a scheme" }] });
    expect(screen.getByRole("alert")).toHaveTextContent("must include a scheme");
  });
});

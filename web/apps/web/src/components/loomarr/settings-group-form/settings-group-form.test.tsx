import type { SettingEntry } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SettingsGroupForm } from "./settings-group-form";

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
  entry({ key: "library.url", kind: "url" }),
  entry({ key: "season.precision", kind: "enum", enum: ["series", "season"], advanced: true }),
];

const renderForm = (props: Partial<Parameters<typeof SettingsGroupForm>[0]> = {}) =>
  render(
    <SettingsGroupForm
      entries={entries}
      values={{ "library.url": "http://x", "season.precision": "series" }}
      onChange={vi.fn()}
      {...props}
    />,
  );

describe("SettingsGroupForm", () => {
  it("hides advanced keys until asked, then reveals them", async () => {
    renderForm();
    expect(screen.queryByLabelText("Season precision")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /show advanced \(1\)/i }));
    expect(screen.getByLabelText("Season precision")).toBeInTheDocument();
  });

  it("routes a field edit to onChange keyed by setting", async () => {
    const onChange = vi.fn();
    renderForm({ onChange });
    await userEvent.type(screen.getByLabelText("Library URL"), "y");
    expect(onChange).toHaveBeenCalledWith("library.url", "http://xy");
  });

  it("runs the group's live check and reports a failure as words", async () => {
    const onTest = vi.fn();
    renderForm({ onTest, testOk: false, testHint: "Emby refused the token." });
    await userEvent.click(screen.getByRole("button", { name: /test connection/i }));
    expect(onTest).toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("Emby refused the token.");
  });

  it("disables the actions while saving", () => {
    renderForm({ onSave: vi.fn(), onTest: vi.fn(), saving: true });
    expect(screen.getByRole("button", { name: /saving/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /test connection/i })).toBeDisabled();
  });

  it("shows a per-key problem from the last save", () => {
    renderForm({ results: [{ key: "library.url", status: "invalid", problem: "must include a scheme" }] });
    expect(screen.getByRole("alert")).toHaveTextContent("must include a scheme");
  });
});

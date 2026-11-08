import type { SettingEntry } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactElement, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { SettingField } from "./setting-field";

// The field's validation and unavailable lines are design-system FieldHints, which need the
// theme the app mounts at the root.
const render = (ui: ReactElement) => rtlRender(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);

const entry = (over: Partial<SettingEntry> = {}): SettingEntry => ({
  key: "library.url",
  group: "connections.media_server",
  owner: "settings.connections",
  kind: "url",
  doc: "Base URL of your Emby/Jellyfin server.",
  advanced: false,
  apply: SettingEntryApply.live,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

// The registry's household schedule, in the canonical form the server publishes it.
const downloadSchedulePresets = [
  { value: "0s", label: "Never" },
  { value: "6h0m0s", label: "Every 6 hours" },
  { value: "12h0m0s", label: "Every 12 hours" },
  { value: "24h0m0s", label: "Daily" },
  { value: "168h0m0s", label: "Weekly" },
];

describe("SettingField", () => {
  it("shows the one-sentence summary under the field and keeps the detail behind the info icon", () => {
    render(
      <SettingField
        entry={entry({ doc: "The address of your media server.", detail: "For example http://emby:8096." })}
        value=""
        onChange={vi.fn()}
      />,
    );
    const input = screen.getByLabelText("Library URL");
    expect(input).toHaveAccessibleDescription("The address of your media server.");
    expect(screen.getByText("The address of your media server.")).toBeVisible();
    const info = screen.getByRole("button", { name: /about library url/i });
    expect(info).toHaveAccessibleDescription("For example http://emby:8096.");
  });

  it("offers no info icon when the summary says it all", () => {
    render(
      <SettingField
        entry={entry({ doc: "The address of your media server." })}
        value=""
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText("The address of your media server.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /about library url/i })).not.toBeInTheDocument();
  });

  it("uses the registry-owned label instead of deriving product copy from the key", () => {
    render(<SettingField entry={entry({ label: "Media server address" })} value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Media server address")).toBeInTheDocument();
  });

  it("edits Go durations in human units while preserving seconds on the wire", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "session.ttl", kind: "duration", label: "Sign-in lifetime" })}
        value="720h"
        onChange={onChange}
      />,
    );
    const amount = screen.getByLabelText("Sign-in lifetime");
    expect(amount).toHaveValue("30");
    expect(screen.getByLabelText("Sign-in lifetime unit")).toHaveTextContent("days");
    fireEvent.change(amount, { target: { value: "14" } });
    expect(onChange).toHaveBeenLastCalledWith("1209600s");
  });

  it("keeps a half-typed decimal in a number box", async () => {
    const Controlled = () => {
      const [bytes, setBytes] = useState(String(2 * 1024 ** 2));
      return (
        <SettingField
          entry={entry({ key: "images.max_upload_bytes", kind: "int", presentation: "bytes" })}
          value={bytes}
          onChange={setBytes}
        />
      );
    };
    render(<Controlled />);
    const amount = screen.getByLabelText("Images max upload bytes");
    await userEvent.clear(amount);
    await userEvent.type(amount, "1.");
    expect(amount).toHaveValue("1.");
    await userEvent.type(amount, "5");
    expect(amount).toHaveValue("1.5");
  });

  it("offers common automatic-download schedules before revealing a custom duration", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({
          key: "filler.fetch.every",
          kind: "duration",
          label: "Look for new clips",
          presentation: "filler_download_schedule",
          presets: downloadSchedulePresets,
        })}
        value="6h0m0s"
        onChange={onChange}
      />,
    );

    const schedule = screen.getByLabelText("Look for new clips");
    expect(schedule).toHaveTextContent("Every 6 hours");
    expect(screen.queryByLabelText("Custom check interval")).not.toBeInTheDocument();

    await userEvent.click(schedule);
    expect(await screen.findByRole("option", { name: "Never" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: "Custom" }));
    expect(screen.getByLabelText("Custom check interval")).toHaveValue("6");
    fireEvent.change(screen.getByLabelText("Custom check interval"), { target: { value: "8" } });
    expect(onChange).toHaveBeenLastCalledWith("28800s");
  });

  it("offers the registry's presets and stores a selected one", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({
          key: "filler.fetch.every",
          kind: "duration",
          label: "Look for new clips",
          presentation: "filler_download_schedule",
          presets: downloadSchedulePresets,
        })}
        value="6h0m0s"
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByLabelText("Look for new clips"));
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      ...downloadSchedulePresets.map((preset) => preset.label),
      "Custom",
    ]);
    await userEvent.click(screen.getByRole("option", { name: "Every 12 hours" }));
    expect(onChange).toHaveBeenLastCalledWith("12h0m0s");
  });

  // The list is the registry's, not a copy: a preset the server adds appears with no UI change,
  // and a value matching none of them reads as Custom.
  it("shows exactly the presets the entry carries", async () => {
    render(
      <SettingField
        entry={entry({
          key: "filler.fetch.every",
          kind: "duration",
          label: "Look for new clips",
          presentation: "filler_download_schedule",
          presets: [{ value: "3h0m0s", label: "Every 3 hours" }],
        })}
        value="5h0m0s"
        onChange={vi.fn()}
      />,
    );

    const schedule = screen.getByLabelText("Look for new clips");
    expect(schedule).toHaveTextContent("Custom");
    expect(screen.getByLabelText("Custom check interval")).toHaveValue("5");
    await userEvent.click(schedule);
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual(["Every 3 hours", "Custom"]);
  });

  it("shows byte limits as MiB and emits bytes", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "images.max_upload_bytes", kind: "int", presentation: "bytes" })}
        value="8388608"
        onChange={onChange}
      />,
    );
    const amount = screen.getByLabelText("Images max upload bytes");
    expect(amount).toHaveValue("8");
    expect(screen.getByText("MiB")).toBeInTheDocument();
    fireEvent.change(amount, { target: { value: "16" } });
    expect(onChange).toHaveBeenLastCalledWith("16777216");
  });

  it("shows a plain number's registry-declared unit beside it and emits the number unchanged", () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "filler.storage.library_budget_gb", kind: "int", unit: "GiB" })}
        value="50"
        onChange={onChange}
      />,
    );
    const amount = screen.getByLabelText("Filler storage library budget gb");
    expect(amount).toHaveValue("50");
    expect(screen.getByText("GiB")).toBeInTheDocument();
    fireEvent.change(amount, { target: { value: "60" } });
    expect(onChange).toHaveBeenLastCalledWith("60");
  });

  it("emits edits as strings", async () => {
    const onChange = vi.fn();
    render(<SettingField entry={entry()} value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Library URL"), "h");
    expect(onChange).toHaveBeenCalledWith("h");
  });

  it("renders a registry-declared boolean switch with accessible state", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "filler.research.enabled", kind: "bool", presentation: "switch" })}
        value="false"
        onChange={onChange}
      />,
    );

    const control = screen.getByRole("switch", { name: "Filler research enabled" });
    expect(control).not.toBeChecked();
    await userEvent.click(control);
    expect(onChange).toHaveBeenCalledWith("true");
  });

  // #2759: a pinned value is read-only, not disabled. Disabled dims it to placeholder grey, and a
  // real value must never be mistaken for an empty hint.
  it("locks an env-pinned field read-only at full strength and says so", () => {
    render(<SettingField entry={entry({ provenance: "env" })} value="http://x" onChange={vi.fn()} />);
    const input = screen.getByLabelText("Library URL");
    expect(input).toHaveAttribute("readonly");
    expect(input).not.toBeDisabled();
    expect(input).toHaveValue("http://x");
    expect(input).not.toHaveStyle({ opacity: "0.62" });
    expect(screen.getByText(/set via environment/i)).toBeInTheDocument();
  });

  // The unlock (config-design §3.1). The dead end this closes: a pinned field is read-only,
  // and before 3.1 the only route forward was editing a file on the host and restarting.

  it("offers no way out when the surface supplies no handler", () => {
    // A read-only context keeps the pre-3.1 behaviour exactly — a plain lock, no affordance.
    render(
      <SettingField
        entry={entry({ provenance: "env", envPinnable: true })}
        value="http://x"
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/set via environment/i)).toBeInTheDocument();
    // The chip is still just a chip — not a button that would do nothing.
    expect(screen.queryByRole("button", { name: /unlock library url/i })).not.toBeInTheDocument();
  });

  // Clicking the LOCK is the unlock: the chip explains why the field is read-only, so the way
  // out belongs on it rather than on a separate control somewhere else in the row.
  it("unlocks by clicking the lock itself", async () => {
    const onEnvOverride = vi.fn();
    render(
      <SettingField
        entry={entry({ provenance: "env", envPinnable: true, envVar: "LIBRARY_URL" })}
        value="http://x"
        onChange={vi.fn()}
        onEnvOverride={onEnvOverride}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /unlock library url/i }));
    expect(onEnvOverride).toHaveBeenCalledWith(true);
  });

  // The visible chip text is the STATE ("set via environment"); the ACTION lives in the
  // accessible name, or a screen-reader user hears "set via environment, button" and has to
  // guess what pressing it does. The name also says which variable is in play.
  it("names the action and the variable for assistive tech, and is keyboard-reachable", async () => {
    const onEnvOverride = vi.fn();
    render(
      <SettingField
        entry={entry({ provenance: "env", envPinnable: true, envVar: "LIBRARY_URL" })}
        value="http://x"
        onChange={vi.fn()}
        onEnvOverride={onEnvOverride}
      />,
    );
    const lock = screen.getByRole("button", {
      name: /unlock library url to edit it here, currently set by LIBRARY_URL/i,
    });
    lock.focus();
    await userEvent.keyboard("{Enter}");
    expect(onEnvOverride).toHaveBeenCalledWith(true);
  });

  // ⚠ An unlocked key resolves as `db` while its variable is still SET, so the field must be
  // editable AND still say what it is overriding. "set via environment" here would contradict
  // an enabled input; a bare db chip would imply the variable was never set at all.
  it("an overriding field is editable and names the variable it overrides", () => {
    render(
      <SettingField
        entry={entry({ provenance: "db", envPinnable: true, envOverride: true, envVar: "LIBRARY_URL" })}
        value="http://x"
        onChange={vi.fn()}
        onEnvOverride={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Library URL")).not.toBeDisabled();
    expect(screen.getByText(/overriding LIBRARY_URL/i)).toBeInTheDocument();
    expect(screen.queryByText(/set via environment/i)).not.toBeInTheDocument();
  });

  it("hands an overriding key back to the environment", async () => {
    const onEnvOverride = vi.fn();
    render(
      <SettingField
        entry={entry({ provenance: "db", envPinnable: true, envOverride: true, envVar: "LIBRARY_URL" })}
        value="http://x"
        onChange={vi.fn()}
        onEnvOverride={onEnvOverride}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /hand library url back to LIBRARY_URL/i }));
    // false, not true: a toggle that always claimed would make the control one-way.
    expect(onEnvOverride).toHaveBeenCalledWith(false);
  });

  it("does not offer the unlock on a key the environment never set", () => {
    render(
      <SettingField
        entry={entry({ provenance: "db" })}
        value="http://x"
        onChange={vi.fn()}
        onEnvOverride={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /unlock/i })).not.toBeInTheDocument();
  });

  it("masks a stored secret until the operator chooses to replace it", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "seerr.api_key", kind: "secret", secret: true, preview: "…a1b2" })}
        value=""
        onChange={onChange}
      />,
    );
    expect(screen.getByText("…a1b2")).toBeInTheDocument();
    expect(screen.queryByLabelText("Seerr API key")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /replace/i }));
    expect(screen.getByLabelText("Seerr API key")).toBeInTheDocument();
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("renders an enum with registry-owned labels but emits the lowercase value", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({
          key: "library.flavor",
          kind: "enum",
          enum: ["emby", "jellyfin"],
          enumOptions: [
            { value: "emby", label: "Emby" },
            { value: "jellyfin", label: "Jellyfin" },
          ],
        })}
        value="jellyfin"
        onChange={onChange}
      />,
    );
    // The trigger shows the capitalized LABEL, not the raw value.
    const trigger = screen.getByLabelText("Library flavor");
    expect(trigger).toHaveTextContent("Jellyfin");

    await userEvent.click(trigger);
    // Options show labels ("Emby"), not "emby".
    expect(await screen.findByRole("option", { name: "Emby" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: "Emby" }));
    // …but the stored value stays lowercase (the BE contract).
    expect(onChange).toHaveBeenCalledWith("emby");
  });

  it("falls back to the raw value when an enum ships no labels", async () => {
    render(
      <SettingField
        entry={entry({ key: "playout.backend", kind: "enum", enum: ["internal", "tunarr"] })}
        value="internal"
        onChange={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByLabelText("Playout backend"));
    expect(await screen.findByRole("option", { name: "tunarr" })).toBeInTheDocument();
  });

  it("toggles a bool as a checkbox", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({
          key: "filler.transcribe.enabled",
          label: "Listen for missing clip details",
          kind: "bool",
        })}
        value="false"
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("checkbox", { name: "Listen for missing clip details" }));
    expect(onChange).toHaveBeenCalledWith("true");
  });

  it("surfaces a per-key validation problem", () => {
    render(
      <SettingField
        entry={entry()}
        value="nope"
        onChange={vi.fn()}
        result={{ key: "library.url", status: "invalid", problem: "must include a scheme" }}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("must include a scheme");
  });

  // The alert is announced once, as it appears. An operator who tabs away and comes back to fix
  // the value hears the problem again only because the field is described by it.
  it("describes an invalid field by its validation problem, not only by an alert", () => {
    render(
      <SettingField
        entry={entry({ label: "Media server address" })}
        value="nope"
        onChange={vi.fn()}
        result={{ key: "library.url", status: "invalid", problem: "must include a scheme" }}
      />,
    );
    const field = screen.getByRole("textbox", { name: "Media server address" });
    expect(field).toBeInvalid();
    expect(field).toHaveAccessibleDescription("Base URL of your Emby/Jellyfin server. must include a scheme");
  });
});

import type { SettingEntry } from "@loomarr/api";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { SettingField } from "./setting-field";

// The field's doc now shows in a FieldHelp (i) tooltip, which needs a TooltipProvider ancestor
// (the app mounts one at the root). Wrap every render so the field mounts without a Radix error.
const render = (ui: ReactElement) => rtlRender(<TooltipProvider>{ui}</TooltipProvider>);

const entry = (over: Partial<SettingEntry> = {}): SettingEntry => ({
  key: "library.url",
  group: "connections.media_server",
  kind: "url",
  doc: "Base URL of your Emby/Jellyfin server.",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

describe("SettingField", () => {
  it("labels the field from its key and exposes the doc via a help affordance", () => {
    render(<SettingField entry={entry()} value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Library URL")).toBeInTheDocument();
    // The doc is now a FieldHelp (i) tooltip in the label row (visible on hover) …
    expect(screen.getByRole("button", { name: /about library url/i })).toBeInTheDocument();
    // … and still in the DOM (visually hidden) for the control's aria-describedby (a11y).
    expect(screen.getByText("Base URL of your Emby/Jellyfin server.")).toBeInTheDocument();
  });

  it("emits edits as strings", async () => {
    const onChange = vi.fn();
    render(<SettingField entry={entry()} value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Library URL"), "h");
    expect(onChange).toHaveBeenCalledWith("h");
  });

  it("locks an env-pinned field and says so", () => {
    render(<SettingField entry={entry({ provenance: "env" })} value="http://x" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Library URL")).toBeDisabled();
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
      name: /unlock library url to edit it here — currently set by LIBRARY_URL/i,
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
        entry={entry({ key: "sched.default_strategy", kind: "enum", enum: ["sequential", "shuffle"] })}
        value="shuffle"
        onChange={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByLabelText("Sched default strategy"));
    expect(await screen.findByRole("option", { name: "sequential" })).toBeInTheDocument();
  });

  it("toggles a bool as a checkbox", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "filler.ai_tagging", kind: "bool" })}
        value="false"
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByLabelText("Filler AI tagging"));
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
});

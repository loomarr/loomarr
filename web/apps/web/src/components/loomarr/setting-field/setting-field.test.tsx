import type { SettingEntry } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SettingField } from "./setting-field";

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
  it("labels the field from its key and shows the doc as help text", () => {
    render(<SettingField entry={entry()} value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Library URL")).toBeInTheDocument();
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

  it("renders an enum as a select of its options", async () => {
    const onChange = vi.fn();
    render(
      <SettingField
        entry={entry({ key: "library.flavor", kind: "enum", enum: ["emby", "jellyfin"] })}
        value="jellyfin"
        onChange={onChange}
      />,
    );
    // The trigger shows the current value; the options are in a listbox opened on click.
    const trigger = screen.getByLabelText("Library flavor");
    expect(trigger).toHaveTextContent("jellyfin");

    await userEvent.click(trigger);
    expect(await screen.findByRole("option", { name: "emby" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: "emby" }));
    expect(onChange).toHaveBeenCalledWith("emby");
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

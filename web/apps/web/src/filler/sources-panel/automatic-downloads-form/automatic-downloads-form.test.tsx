import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import {
  AutomaticDownloadsForm,
  downloadIntervalMaximum,
  downloadIntervalParts,
  downloadIntervalSeconds,
  downloadSchedulePreset,
} from "./automatic-downloads-form";

const archive = (automaticDownloads?: NonNullable<ReturnType<typeof fillerSource>["automaticDownloads"]>) =>
  fillerSource({
    id: "archive:classic",
    kind: "archive",
    automaticDownloads: automaticDownloads ?? {
      mode: "defaults",
      everySeconds: 21600,
      maxPerCheck: 10,
      summary: "Uses your defaults: every 6 hours, up to 10 clips each check.",
    },
  });

const idle = { updating: false, updatingSourceID: undefined };

describe("download interval helpers", () => {
  it("shows an interval in the largest unit that divides it evenly", () => {
    expect(downloadIntervalParts(86400 * 2)).toEqual({ amount: 2, unit: "days" });
    expect(downloadIntervalParts(3600 * 5)).toEqual({ amount: 5, unit: "hours" });
    expect(downloadIntervalParts(90 * 60)).toEqual({ amount: 90, unit: "minutes" });
    expect(downloadIntervalParts(10)).toEqual({ amount: 1, unit: "minutes" });
  });

  it("never turns an amount below one into a shorter interval", () => {
    expect(downloadIntervalSeconds(0, "hours")).toBe(3600);
    expect(downloadIntervalSeconds(1.5, "hours")).toBe(5400);
  });

  it("caps each unit at one week", () => {
    expect(downloadIntervalMaximum("days")).toBe(7);
    expect(downloadIntervalMaximum("hours")).toBe(168);
    expect(downloadIntervalMaximum("minutes")).toBe(10080);
  });

  it("recognises the four presets and calls anything else custom", () => {
    expect(downloadSchedulePreset(6 * 3600)).toBe("6h");
    expect(downloadSchedulePreset(12 * 3600)).toBe("12h");
    expect(downloadSchedulePreset(24 * 3600)).toBe("daily");
    expect(downloadSchedulePreset(7 * 24 * 3600)).toBe("weekly");
    expect(downloadSchedulePreset(5 * 3600)).toBe("custom");
  });
});

describe("AutomaticDownloadsForm", () => {
  it("keeps the schedule hidden until the source chooses its own", async () => {
    render(<AutomaticDownloadsForm source={archive()} actions={{ ...idle, updateSource: vi.fn() }} />);

    expect(screen.getByText(/Uses your defaults: every 6 hours/)).toBeVisible();
    expect(screen.queryByLabelText("Source check interval")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("combobox", { name: "Automatic downloads for this source" }));
    await userEvent.click(await screen.findByRole("option", { name: "Use a different schedule" }));

    expect(screen.getByRole("combobox", { name: "Source check schedule" })).toBeVisible();
    expect(screen.getByLabelText("Clips per source check")).toHaveValue(10);
  });

  it("saves a custom schedule with the source's enabled flag", async () => {
    const updateSource = vi.fn();
    render(<AutomaticDownloadsForm source={archive()} actions={{ ...idle, updateSource }} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Automatic downloads for this source" }));
    await userEvent.click(await screen.findByRole("option", { name: "Use a different schedule" }));
    await userEvent.click(screen.getByRole("combobox", { name: "Source check schedule" }));
    await userEvent.click(await screen.findByRole("option", { name: "Custom" }));
    fireEvent.change(screen.getByLabelText("Source check interval"), { target: { value: "12" } });
    fireEvent.change(screen.getByLabelText("Clips per source check"), { target: { value: "3" } });
    await userEvent.click(screen.getByRole("button", { name: "Save automatic downloads" }));

    expect(updateSource).toHaveBeenCalledWith(
      "archive:classic",
      { enabled: true, automaticDownloads: { mode: "custom", everySeconds: 43200, maxPerCheck: 3 } },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });

  it("sends only the mode when the source stops downloading automatically", async () => {
    const updateSource = vi.fn();
    render(<AutomaticDownloadsForm source={archive()} actions={{ ...idle, updateSource }} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Automatic downloads for this source" }));
    await userEvent.click(await screen.findByRole("option", { name: "Never download automatically" }));
    await userEvent.click(screen.getByRole("button", { name: "Save automatic downloads" }));

    expect(updateSource).toHaveBeenCalledWith(
      "archive:classic",
      { enabled: true, automaticDownloads: { mode: "never" } },
      expect.anything(),
    );
  });

  it("keeps a save failure beside the controls", async () => {
    const updateSource = vi.fn((_id, _data, callbacks) =>
      callbacks.onError({ detail: "Schedule rejected." }),
    );
    render(<AutomaticDownloadsForm source={archive()} actions={{ ...idle, updateSource }} />);

    await userEvent.click(screen.getByRole("button", { name: "Save automatic downloads" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Schedule rejected.");
  });

  it("labels the button while this source is saving and locks it while any source is", () => {
    render(
      <AutomaticDownloadsForm
        source={archive()}
        actions={{ updating: true, updatingSourceID: "archive:classic", updateSource: vi.fn() }}
      />,
    );

    expect(screen.getByRole("button", { name: "Saving…" })).toBeDisabled();
  });

  it("renders nothing for a source with no download policy", () => {
    const { container } = render(
      <AutomaticDownloadsForm source={fillerSource()} actions={{ ...idle, updateSource: vi.fn() }} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});

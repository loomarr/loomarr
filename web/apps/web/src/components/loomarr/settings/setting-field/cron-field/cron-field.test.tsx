import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { CronField } from "./cron-field";

const presets = [{ value: "0 0 * * * *", label: "Every hour" }];

// SettingField renders the visible <label> for the control's id; this stands in for it.
const field = (value: string, onChange = vi.fn()) => (
  <>
    <label htmlFor="backup">Backup schedule</label>
    <CronField id="backup" label="Backup schedule" value={value} presets={presets} onChange={onChange} />
  </>
);

describe("CronField", () => {
  it("shows a once-a-day schedule as a time and edits the minute", async () => {
    const onChange = vi.fn();
    render(field("0 30 3 * * *", onChange));

    expect(screen.getByLabelText("Backup schedule")).toHaveTextContent("Every day at a time");
    expect(screen.getByLabelText("Backup schedule minute")).toHaveTextContent(":30");
    expect(screen.queryByDisplayValue("0 30 3 * * *")).not.toBeInTheDocument();

    await userEvent.click(screen.getByLabelText("Backup schedule minute"));
    await userEvent.click(await screen.findByRole("option", { name: ":45" }));
    expect(onChange).toHaveBeenLastCalledWith("0 45 3 * * *");
  });

  it("stores a named schedule's own value", async () => {
    const onChange = vi.fn();
    render(field("0 30 3 * * *", onChange));
    await userEvent.click(screen.getByLabelText("Backup schedule"));
    await userEvent.click(await screen.findByRole("option", { name: "Every hour" }));
    expect(onChange).toHaveBeenLastCalledWith("0 0 * * * *");
  });

  it("keeps a schedule that is not once a day behind Custom schedule", () => {
    render(field("0 30 3 * * 1"));
    expect(screen.getByLabelText("Backup schedule")).toHaveTextContent("Custom schedule");
    expect(screen.getByLabelText("Backup schedule custom schedule")).toHaveValue("0 30 3 * * 1");
  });
});

import { LoomarrProvider } from "@loomarr/design-system";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DurationField } from "./duration-field";

const renderField = (seconds: number, onSeconds = vi.fn()) => {
  render(
    <LoomarrProvider theme="dark">
      <DurationField
        amountLabel="Spacing"
        label="Spacing"
        onSeconds={onSeconds}
        seconds={seconds}
        units={["days", "hours", "minutes", "seconds"]}
      />
    </LoomarrProvider>,
  );
  return onSeconds;
};

describe("DurationField", () => {
  it("opens in the largest unit that holds the amount whole", () => {
    renderField(5400);
    expect(screen.getByLabelText("Spacing")).toHaveValue("90");
    expect(screen.getByLabelText("Spacing unit")).toHaveValue("minutes");
  });

  it("opens a zero in the smallest unit", () => {
    renderField(0);
    expect(screen.getByLabelText("Spacing unit")).toHaveValue("seconds");
  });

  it("reports the typed amount in seconds of the chosen unit", () => {
    const onSeconds = renderField(3600);
    fireEvent.change(screen.getByLabelText("Spacing"), { target: { value: "2" } });
    expect(onSeconds).toHaveBeenLastCalledWith(7200);
    fireEvent.change(screen.getByLabelText("Spacing unit"), { target: { value: "days" } });
    expect(onSeconds).toHaveBeenLastCalledWith(86400);
  });
});

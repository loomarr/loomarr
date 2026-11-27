import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { CopyValue } from "./copy-value";

describe("CopyValue", () => {
  it("names itself with the value it shows and copies that value", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    render(<CopyValue label="Run ID" value="run-42" />);

    await userEvent.click(screen.getByRole("button", { name: "Copy Run ID: run-42" }));

    expect(writeText).toHaveBeenCalledWith("run-42");
    expect(screen.getByText("run-42")).toBeVisible();
  });

  it("copies by selection on a plain-HTTP origin instead of throwing", async () => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, "execCommand", { configurable: true, value: execCommand });
    render(<CopyValue label="Run ID" value="run-42" />);

    await userEvent.click(screen.getByRole("button", { name: "Copy Run ID: run-42" }));

    expect(execCommand).toHaveBeenCalledWith("copy");
    Reflect.deleteProperty(document, "execCommand");
  });
});

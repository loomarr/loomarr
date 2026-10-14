import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { LanguagePicker } from "./language-picker";

it("filters friendly language names and chooses the active result from the keyboard", async () => {
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(<LanguagePicker value="en" options={["en", "pl"]} onChange={onChange} />);

  const picker = screen.getByRole("combobox", { name: "Commercial language" });
  await user.clear(picker);
  await user.type(picker, "poli{Enter}");

  expect(onChange).toHaveBeenCalledWith("pl");
  expect(picker).toHaveValue("Polish");
  expect(screen.getByText(/wordless clips stay/i)).toBeInTheDocument();
});

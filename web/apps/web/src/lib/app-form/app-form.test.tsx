import { LoomarrProvider } from "@loomarr/design-system";
import { render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { z } from "zod";
import { useAppForm } from "./app-form";

const render = (ui: ReactNode) => rtlRender(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);

const schema = z.object({ name: z.string().min(1, "Enter a name.") });

const Example = ({ onSubmit }: { onSubmit: (name: string) => void }) => {
  const form = useAppForm({
    defaultValues: { name: "" },
    validators: { onSubmit: schema },
    onSubmit: ({ value }) => onSubmit(value.name),
  });
  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        void form.handleSubmit();
      }}
    >
      <form.AppField name="name">{(field) => <field.TextField label="Name" />}</form.AppField>
      <button type="submit">Save</button>
    </form>
  );
};

const FocusExample = () => {
  const form = useAppForm({ defaultValues: { name: "" } });
  return <form.AppField name="name">{(field) => <field.TextField autoFocus label="Name" />}</form.AppField>;
};

describe("useAppForm's TextField", () => {
  it("takes focus on mount when asked to", () => {
    render(<FocusExample />);
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveFocus();
  });

  it("labels the input and submits what was typed", async () => {
    const onSubmit = vi.fn();
    render(<Example onSubmit={onSubmit} />);
    await userEvent.type(screen.getByRole("textbox", { name: "Name" }), "Kitchen TV");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenCalledWith("Kitchen TV");
  });

  it("marks the input invalid and shows the message on submit, then clears it on typing", async () => {
    render(<Example onSubmit={vi.fn()} />);
    const input = screen.getByRole("textbox", { name: "Name" });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a name.");
    expect(input).toHaveAttribute("aria-invalid", "true");

    // Leaving the field keeps the message: only typing is a reason to clear it.
    await userEvent.tab();
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a name.");

    await userEvent.type(input, "K");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(input).not.toHaveAttribute("aria-invalid");
  });
});

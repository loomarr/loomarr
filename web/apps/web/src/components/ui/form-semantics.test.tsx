import { FieldHint, LoomarrProvider, semanticColors } from "@loomarr/design-system";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as React from "react";
import { describe, expect, it, vi } from "vitest";
import { Checkbox } from "./checkbox";
import { Input } from "./input";
import { Label } from "./label";
import { Switch } from "./switch";

// The form controls moved onto the shared design-system tokens and FieldHint (#970 step 45). What
// must not move with them is the browser's own form behaviour: these cross the same interface a
// form does (label, keyboard, constraint validation, submission) rather than pinning markup.

const renderForm = (ui: React.ReactNode) => {
  const onSubmit = vi.fn((event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    return new FormData(event.currentTarget);
  });
  render(
    <LoomarrProvider theme="dark">
      <form aria-label="settings" onSubmit={onSubmit}>
        {ui}
        <button type="submit">Save</button>
      </form>
    </LoomarrProvider>,
  );
  return { form: screen.getByRole("form", { name: "settings" }) as HTMLFormElement, onSubmit };
};

const submitted = (onSubmit: ReturnType<typeof renderForm>["onSubmit"]) =>
  onSubmit.mock.results.at(-1)?.value as FormData;

describe("form field anatomy on the shared design system", () => {
  it("names, describes and flags a text field through its label and hints", () => {
    renderForm(
      <>
        <Label htmlFor="server-url">Server address</Label>
        <Input id="server-url" name="url" aria-invalid aria-describedby="server-url-doc server-url-error" />
        <FieldHint id="server-url-doc">The address other devices use to reach this server.</FieldHint>
        <FieldHint id="server-url-error" role="alert" tone="error">
          Enter a full URL.
        </FieldHint>
      </>,
    );

    const field = screen.getByRole("textbox", { name: "Server address" });
    expect(field).toBeInvalid();
    expect(field).toHaveAccessibleDescription(
      "The address other devices use to reach this server. Enter a full URL.",
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a full URL.");
  });

  it("keeps the browser's required validation and blocks an empty submission", async () => {
    const { form, onSubmit } = renderForm(
      <>
        <Label htmlFor="channel-name">Channel name</Label>
        <Input id="channel-name" name="name" required />
      </>,
    );
    const field = screen.getByRole("textbox", { name: "Channel name" }) as HTMLInputElement;

    expect(field).toBeRequired();
    expect(form.checkValidity()).toBe(false);
    expect(field.validity.valueMissing).toBe(true);
    expect(field.validationMessage).not.toBe("");

    await userEvent.type(field, "Morning Shorts");
    expect(form.checkValidity()).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submitted(onSubmit).get("name")).toBe("Morning Shorts");
  });

  it("submits a password field's value whether masked or revealed", async () => {
    const { onSubmit } = renderForm(
      <>
        <Label htmlFor="pw">Password</Label>
        <Input id="pw" name="password" type="password" defaultValue="correct-horse" />
      </>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Show password" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submitted(onSubmit).get("password")).toBe("correct-horse");
  });

  it("toggles a checkbox from its label and from Space, and submits it only when checked", async () => {
    const { onSubmit } = renderForm(
      <>
        <Checkbox id="purge" name="purge" value="yes" />
        <Label htmlFor="purge">Also remove it from Tunarr</Label>
      </>,
    );
    const box = screen.getByRole("checkbox", { name: "Also remove it from Tunarr" });

    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submitted(onSubmit).has("purge")).toBe(false);

    await userEvent.click(screen.getByText("Also remove it from Tunarr"));
    expect(box).toBeChecked();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submitted(onSubmit).get("purge")).toBe("yes");

    box.focus();
    await userEvent.keyboard(" ");
    expect(box).not.toBeChecked();
  });

  it("keeps a required checkbox's browser validation", () => {
    const { form } = renderForm(
      <>
        <Checkbox id="terms" name="terms" required />
        <Label htmlFor="terms">I have a backup</Label>
      </>,
    );

    expect(screen.getByRole("checkbox", { name: "I have a backup" })).toBeRequired();
    expect(form.checkValidity()).toBe(false);
  });

  it("names a switch through aria-labelledby and submits its state", async () => {
    const SwitchField = () => {
      const [on, setOn] = React.useState(false);
      return (
        <>
          <span id="source-label">Use this source</span>
          <Switch
            name="enabled"
            checked={on}
            onChange={(event) => setOn(event.target.checked)}
            aria-labelledby="source-label"
          />
        </>
      );
    };
    const { onSubmit } = renderForm(<SwitchField />);
    const toggle = screen.getByRole("switch", { name: "Use this source" });

    toggle.focus();
    await userEvent.keyboard(" ");
    expect(toggle).toBeChecked();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submitted(onSubmit).get("enabled")).toBe("on");
  });

  // The values are the design system's, not copies of them: a token change reaches the forms.
  it("paints the field and checkbox from the shared semantic tokens", () => {
    renderForm(
      <>
        <Input aria-label="Search" />
        <Checkbox aria-label="Pick" />
      </>,
    );

    const field = screen.getByRole("textbox", { name: "Search" });
    expect(field.style.borderColor).toBe(hexToRgb(semanticColors.border.control));
    expect(field.style.color).toBe(hexToRgb(semanticColors.content.primary));
    expect(field.style.caretColor).toBe(hexToRgb(semanticColors.action.primary));
    expect(screen.getByRole("checkbox", { name: "Pick" }).style.accentColor).toBe(
      hexToRgb(semanticColors.action.primary),
    );
  });

  // A caller's `style` still wins over the defaults, as a caller's class did before.
  it("lets a caller's style override the token defaults", () => {
    render(<Input aria-label="Code" style={{ borderColor: "rgb(1, 2, 3)" }} />);
    expect(screen.getByRole("textbox", { name: "Code" }).style.borderColor).toBe("rgb(1, 2, 3)");
  });
});

const hexToRgb = (hex: string) => {
  const n = Number.parseInt(hex.slice(1, 7), 16);
  return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`;
};

import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FieldError } from "./field-error";

describe("FieldError", () => {
  it("announces the first issue a field has", () => {
    render(
      <LoomarrProvider theme="dark">
        <FieldError errors={[undefined, { message: "Pick a username." }, { message: "Too long." }]} />
      </LoomarrProvider>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Pick a username.");
  });

  it("renders nothing for a field that passed", () => {
    const { container } = render(
      <LoomarrProvider theme="dark">
        <FieldError errors={[undefined]} />
      </LoomarrProvider>,
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(container).toHaveTextContent("");
  });
});

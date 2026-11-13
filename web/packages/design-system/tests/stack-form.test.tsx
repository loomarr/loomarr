// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoomarrProvider, Stack } from "../index";

afterEach(cleanup);

describe("Stack as a form", () => {
  it("renders a <form> that carries its attributes and lays out as a column", () => {
    render(
      <LoomarrProvider theme="dark">
        <Stack aria-label="Sign in" autoComplete="off" gap="4" name="sign-in" noValidate render="form">
          <input aria-label="Name" />
        </Stack>
      </LoomarrProvider>,
    );

    const form = screen.getByRole("form", { name: "Sign in" });
    expect(form.tagName).toBe("FORM");
    expect(form).toHaveProperty("noValidate", true);
    expect(form.getAttribute("autocomplete")).toBe("off");
    expect(form.getAttribute("name")).toBe("sign-in");
    expect(getComputedStyle(form).flexDirection).toBe("column");
  });

  it("hands a submit to onSubmit, so the form's own handler decides what happens", () => {
    const onSubmit = vi.fn((event: { preventDefault: () => void }) => event.preventDefault());
    render(
      <LoomarrProvider theme="dark">
        <Stack aria-label="Search" noValidate onSubmit={onSubmit} render="form">
          <button type="submit">Go</button>
        </Stack>
      </LoomarrProvider>,
    );

    fireEvent.submit(screen.getByRole("form", { name: "Search" }));

    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("takes form attributes only when it renders a form", () => {
    const section = (
      // @ts-expect-error `noValidate` belongs to a form; a section has no use for it.
      <Stack noValidate render="section" />
    );
    const bare = (
      // @ts-expect-error without `render="form"` the Stack is not a form either.
      <Stack onSubmit={() => undefined} />
    );

    expect(section).toBeDefined();
    expect(bare).toBeDefined();
  });
});

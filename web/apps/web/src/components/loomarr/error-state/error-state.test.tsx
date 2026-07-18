import { ApiError } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ErrorState, toProblem } from "./error-state";

describe("ErrorState", () => {
  it("renders an RFC7807 problem's title and detail (never raw JSON)", () => {
    render(
      <ErrorState error={new ApiError(502, { title: "Tunarr unreachable", detail: "connection refused" })} />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Tunarr unreachable");
    expect(alert).toHaveTextContent("connection refused");
    expect(alert.textContent).not.toContain("{");
  });

  it("offers retry only when a handler is given", async () => {
    const onRetry = vi.fn();
    const { rerender } = render(<ErrorState error={new Error("boom")} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    rerender(<ErrorState error={new Error("boom")} onRetry={onRetry} />);
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("toProblem falls back for plain and unknown errors", () => {
    expect(toProblem(new Error("plain")).title).toBe("plain");
    expect(toProblem("weird").title).toBe("Something went wrong");
  });
});

import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RoutePending } from "./route-pending";

describe("RoutePending", () => {
  it("names the wait", () => {
    render(
      <LoomarrProvider theme="dark">
        <RoutePending />
      </LoomarrProvider>,
    );
    expect(screen.getByText("Loading")).toBeInTheDocument();
  });
});

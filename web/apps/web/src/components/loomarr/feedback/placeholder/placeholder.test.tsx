import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Placeholder } from "./placeholder";

describe("Placeholder", () => {
  it("renders the page title and hint", () => {
    render(<Placeholder title="Channels" hint="No channels yet." />);
    expect(screen.getByRole("heading", { name: "Channels" })).toBeInTheDocument();
    expect(screen.getByText("No channels yet.")).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "./story-utils";

describe("RouterHarness", () => {
  it("renders content at a registered navigation root's child path", async () => {
    render(<RouterHarness initialPath="/filler/settings/details" content={<p>Clip details</p>} />);

    expect(await screen.findByText("Clip details")).toBeInTheDocument();
    expect(screen.queryByText("Not Found")).not.toBeInTheDocument();
  });
});

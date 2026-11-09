import { CardAction } from "@loomarr/ui";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { REQUEST_ROW_TONE, RequestRow } from "./request-row";

describe("RequestRow", () => {
  it("is one row card: the title, the status first on the metadata line, and one action", () => {
    render(
      <RequestRow
        action={<CardAction>Open</CardAction>}
        meta={["Requested 2 days ago"]}
        status={{ line: "Waiting for approval", tone: "suggest" }}
        title="Rainy-day detective stories"
      />,
    );
    expect(screen.getByText("Rainy-day detective stories")).toBeInTheDocument();
    expect(screen.getByText("Waiting for approval")).toBeInTheDocument();
    expect(screen.getByText("Requested 2 days ago")).toBeInTheDocument();
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });

  it("draws every request status as a dot tone", () => {
    expect(REQUEST_ROW_TONE).toEqual({
      caution: "progress",
      lock: "ok",
      onair: "error",
      suggest: "attention",
    });
  });
});

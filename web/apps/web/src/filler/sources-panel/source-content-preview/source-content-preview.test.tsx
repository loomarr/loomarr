import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceContentPreview } from "./source-content-preview";

describe("SourceContentPreview", () => {
  it("holds the example section in place while its three preview links load", () => {
    const { rerender } = render(
      <SourceContentPreview
        kind="youtube"
        canonicalUrl="https://www.youtube.com/channel/example/videos"
        loading
      />,
    );

    expect(screen.getByText("A few videos from this source")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading examples…");
    expect(screen.getByRole("status").querySelector(".animate-spin")).not.toBeNull();

    rerender(
      <SourceContentPreview
        kind="youtube"
        canonicalUrl="https://www.youtube.com/channel/example/videos"
        previewItems={[
          { title: "First", url: "https://www.youtube.com/watch?v=first" },
          { title: "Second", url: "https://www.youtube.com/watch?v=second" },
          { title: "Third", url: "https://www.youtube.com/watch?v=third" },
        ]}
      />,
    );

    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Preview" })).toHaveLength(3);
  });
});

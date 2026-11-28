// @vitest-environment jsdom

import { Text } from "@loomarr/design-system";
import { afterEach, describe, expect, it } from "vitest";

import { declarationsOfElement } from "../css";
import { cleanup, renderUi, screen } from "../index";

afterEach(cleanup);

describe("renderUi", () => {
  it("mounts the node under the Loomarr theme, and keeps it across a rerender", () => {
    const { rerender } = renderUi(<Text textRole="body">first</Text>);
    expect(declarationsOfElement(screen.getByText("first"))).toContain("color:");

    rerender(<Text textRole="body">second</Text>);
    expect(declarationsOfElement(screen.getByText("second"))).toContain("color:");
    expect(screen.queryByText("first")).toBeNull();
  });

  it("applies the theme the caller asks for", () => {
    renderUi(<Text textRole="body">lit</Text>, { theme: "light" });
    expect(screen.getByText("lit").closest(".t_light")).not.toBeNull();
  });

  it("is unmounted by cleanup", () => {
    renderUi(<Text textRole="body">gone</Text>);
    cleanup();
    expect(screen.queryByText("gone")).toBeNull();
  });
});

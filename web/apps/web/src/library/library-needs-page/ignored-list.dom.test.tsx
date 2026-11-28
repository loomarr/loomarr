// @vitest-environment jsdom

import { cleanup, renderUi } from "@loomarr/test-support";
import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { IgnoredList } from "./library-needs-page";

afterEach(cleanup);

const items = [
  {
    id: "90000813",
    kind: "movie" as const,
    name: "title_t03.mkv",
    folder: "Discs",
    ignoredAt: "2026-01-02T03:04:05Z",
  },
];

describe("the Ignored list", () => {
  it("disables the row whose Bring back is on its way", () => {
    renderUi(<IgnoredList bringing={new Set(["90000813"])} items={items} onBringBack={() => {}} />);
    expect(
      screen.getByRole("button", { name: "Bring back title_t03.mkv" }).getAttribute("aria-disabled"),
    ).toBe("true");
  });

  // The typecheck holds this: a caller that leaves `bringing` out does not compile.
  it("cannot be drawn without saying what is on its way", () => {
    // @ts-expect-error: `bringing` is required.
    const missing = <IgnoredList items={items} onBringBack={() => {}} />;
    expect(missing.props.bringing).toBeUndefined();
  });
});

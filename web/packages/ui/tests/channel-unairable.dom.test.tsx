// @vitest-environment jsdom

import { cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it } from "vitest";

import { ChannelUnairable } from "../index";

afterEach(cleanup);

describe("ChannelUnairable", () => {
  it("names each title that can't air and why", () => {
    const { container } = renderUi(
      <ChannelUnairable
        titles={[
          { key: "series:tvdb:90000012", reason: "no_episodes_in_seasons", title: "Early Seasons Only" },
          { key: "movie:tmdb:90000002", reason: "not_in_library", title: "Missing Film" },
        ]}
      />,
    );
    const text = container.textContent ?? "";
    expect(text).toContain("2 titles can't air");
    expect(text).toContain("Early Seasons Only has no episodes in the seasons this channel asks for.");
    expect(text).toContain("Missing Film isn't in your library.");
  });

  it("shows nothing when every title airs", () => {
    renderUi(<ChannelUnairable titles={[]} />);
    // The whole body, so text drawn through a portal counts too.
    expect(document.body.textContent).toBe("");
  });
});

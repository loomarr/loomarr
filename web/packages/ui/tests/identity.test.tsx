import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ChannelIdentity, ProgrammeIdentity } from "../index";

describe("shared channel and programme identity", () => {
  it("keeps channel number, name, and a deterministic missing-logo fallback together", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ChannelIdentity
          channel={{ channelLogoState: "missing", channelName: "Classic Animation", channelNumber: "07" }}
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("07");
    expect(markup).toContain("Classic Animation");
    expect(markup).toContain("CA");
  });

  it("publishes the complete episode and airing hierarchy without manufacturing absent fields", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="light">
        <ProgrammeIdentity
          programme={{
            badge: { label: "On now", tone: "live" },
            description: "A classic Puddleford episode.",
            episodeLabel: "S04E12",
            seriesTitle: "Kettle Kids",
            timeLabel: "7:00–7:30 PM",
            title: "The Monorail Pitch",
          }}
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Kettle Kids");
    expect(markup).toContain("The Monorail Pitch");
    expect(markup).toContain("7:00–7:30 PM · S04E12");
    expect(markup).toContain("A classic Puddleford episode.");
    expect(markup).toContain("On now");
  });
});

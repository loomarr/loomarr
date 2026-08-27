import { LoomarrProvider } from "@loomarr/design-system";
import { surfGroups } from "@loomarr/fixtures";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { SurfRail, surfIdentityLabel } from "../index";

const renderRail = (serverVersion?: string, density: "pointer" | "tv" = "tv", onDisconnect?: () => void) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <SurfRail
        clientName="Loomarr TV"
        clientVersion="0.2.0"
        currentChannelId="ch-springfield"
        density={density}
        groups={surfGroups}
        onDisconnect={onDisconnect}
        onFocusSelection={vi.fn()}
        onTune={vi.fn()}
        selection={{ channelId: "ch-springfield", group: "recent" }}
        serverVersion={serverVersion}
      />
    </LoomarrProvider>,
  );

describe("SurfRail", () => {
  it("matches the grouped Compose TV rail and keeps remote hints quiet", () => {
    const output = renderRail("0.2.1");
    expect(output).toContain("FAVOURITES");
    expect(output).toContain("No favourites yet");
    expect(output).toContain("RECENT · 1");
    expect(output).toContain("ALL CHANNELS · 3");
    expect(output).toContain("Springfield Classics");
    expect(output).toContain("Radioactive Man");
    expect(output).toContain("1 of 4 · ▲▼ browse");
    expect(output).toContain("OK tune · BACK cancel");
    expect(output).toContain("Loomarr TV 0.2.0 · Server 0.2.1");
    expect(output).not.toContain(">Channel surfer<");
    expect(output).not.toContain("LIVE TV");
  });

  it("keeps the richer artwork and now/next composition for pointer clients", () => {
    const output = renderRail("0.2.1", "pointer");
    expect(output).toContain("Channel surfer");
    expect(output).toContain("S07E02");
    expect(output).toContain("Next 7:30 PM · Home Sweet Homediddly-Dum-Doodily");
  });

  it("states unavailable server identity honestly", () => {
    expect(renderRail()).toContain("Loomarr TV 0.2.0 · Server unavailable");
  });

  it("keeps confirmed self-disconnect reachable after the TV parity migration", () => {
    expect(renderRail("0.2.1", "tv", vi.fn())).toContain("Disconnect device");
    expect(renderRail("0.2.1")).not.toContain("Disconnect device");
  });

  it("keeps the same identity available to empty and error Surf states", () => {
    expect(surfIdentityLabel("Loomarr TV", "0.2.0", "dev (modified)")).toBe(
      "Loomarr TV 0.2.0 · Server dev (modified)",
    );
  });
});

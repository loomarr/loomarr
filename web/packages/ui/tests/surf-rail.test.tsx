import { monogramOf } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { surfGroups } from "@loomarr/fixtures";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { SurfRail } from "../index";

const renderRail = (serverVersion?: string) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <SurfRail
        clientVersion="0.2.0"
        groups={surfGroups}
        onFocusSelection={vi.fn()}
        onTune={vi.fn()}
        selection={{ channelId: "ch-springfield", group: "recent" }}
        serverVersion={serverVersion}
      />
    </LoomarrProvider>,
  );

describe("SurfRail", () => {
  it("keeps optional groups visible and renders authoritative now/next identity", () => {
    const output = renderRail("0.2.1");
    expect(output).toContain("FAVOURITES");
    expect(output).toContain("No favourites yet");
    expect(output).toContain("Springfield Classics");
    expect(output).toContain("Radioactive Man");
    expect(output).toContain("S07E02");
    expect(output).toContain("Next 7:30 PM · Home Sweet Homediddly-Dum-Doodily");
    expect(output).toContain("Loomarr TV 0.2.0 · Server 0.2.1");
  });

  it("states unavailable server identity honestly", () => {
    expect(renderRail()).toContain("Loomarr TV 0.2.0 · Server unavailable");
  });

  it("preserves the Shield grouped rail and expands the focused TV row", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <SurfRail
          clientVersion="prototype"
          currentChannelId="ch-springfield"
          density="tv"
          groups={surfGroups}
          onFocusSelection={vi.fn()}
          onDisconnect={vi.fn()}
          onTune={vi.fn()}
          selection={{ channelId: "ch-springfield", group: "recent" }}
          serverName="http://loomarr.test:8080"
        />
      </LoomarrProvider>,
    );

    expect(output).toContain("FAVOURITES · 0");
    expect(output).toContain("RECENT · 1");
    expect(output).toContain("ALL CHANNELS · 3");
    expect(output).toContain("Radioactive Man");
    expect(output).toContain("1 of 4 · ▲▼ browse");
    expect(output).toContain("OK tune · BACK cancel");
    expect(output).toContain("Disconnect device");
  });

  it("puts the channel-number wait row above Disconnect on the TV rail (#1659 decision N4)", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <SurfRail
          autoTune={{ choicesMs: [1_200, 5_000], onChange: vi.fn(), valueMs: 5_000 }}
          clientVersion="prototype"
          density="tv"
          groups={surfGroups}
          onDisconnect={vi.fn()}
          onFocusSelection={vi.fn()}
          onTune={vi.fn()}
          selection={{ channelId: "ch-springfield", group: "recent" }}
        />
      </LoomarrProvider>,
    );

    const row = output.indexOf("Channel-number wait · 5 s");
    expect(row).toBeGreaterThan(-1);
    expect(row).toBeLessThan(output.indexOf("Disconnect device"));
  });

  it("draws each channel's monogram ident on the TV row (#1659 decision N8)", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <SurfRail
          clientVersion="prototype"
          currentChannelId="ch-springfield"
          density="tv"
          groups={surfGroups}
          onFocusSelection={vi.fn()}
          onTune={vi.fn()}
          selection={{ channelId: "ch-springfield", group: "recent" }}
        />
      </LoomarrProvider>,
    );
    expect(output).toContain(`>${monogramOf("Springfield Classics")}<`);
  });
});

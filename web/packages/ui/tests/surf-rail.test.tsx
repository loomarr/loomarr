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
});

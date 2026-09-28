import { monogramOf } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ChannelIdent } from "../index";

const render = (element: React.ReactElement) =>
  renderToStaticMarkup(<LoomarrProvider>{element}</LoomarrProvider>);

const NAME = "Late Night Sci-Fi";

// A channel with no icon shows its monogram ident (N8), at the guide's row size or larger.
describe("ChannelIdent", () => {
  it("draws the channel's monogram in a decorative tile, the guide's 30 px by default", () => {
    const markup = render(<ChannelIdent name={NAME} number={7} />);
    expect(markup).toContain(`>${monogramOf(NAME)}<`);
    expect(markup).toContain('aria-hidden="true"');
    expect(markup).toContain("30px");
  });

  it("scales the tile and its monogram together", () => {
    const markup = render(<ChannelIdent name={NAME} number={7} size={48} />);
    expect(markup).toContain("48px");
    // 11 px type at 30 px, so 17.6 px at 48.
    expect(markup).toContain("17.6px");
  });
});

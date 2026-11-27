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
    // 12 px type (the Label role) at 30 px, so 19.2 px at 48.
    expect(markup).toContain("font-size:19.2");
  });

  // The tile is a flat tint (#2762), and its colour keys on the NUMBER, so renaming a channel
  // does not change how the rail looks.
  it("is a flat tint that keeps its colour when only the name changes", () => {
    const tile = (name: string) => {
      const markup = render(<ChannelIdent name={name} number={3} />);
      return markup.match(/background-color:[^;"]*/)?.[0];
    };
    expect(tile("Old Name")).toBeDefined();
    expect(tile("Old Name")).toBe(tile("Completely Different"));
    expect(render(<ChannelIdent name={NAME} number={3} />)).not.toContain("background-image");
  });

  it("draws the channel's icon in the tile, labelled with its name, instead of the monogram", () => {
    const markup = render(<ChannelIdent name={NAME} number={7} logo="/icon.png" size={44} />);
    expect(markup).toContain(`aria-label="${NAME} logo"`);
    expect(markup).toContain("44px");
    expect(markup).not.toContain(`>${monogramOf(NAME)}<`);
  });
});

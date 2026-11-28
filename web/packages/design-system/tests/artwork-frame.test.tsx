import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { declarationsOf } from "../atomic-css";
import { ArtworkFrame, LoomarrProvider, Text } from "../index";

// The frame's shape is the caller's choice: 16:9 unless it asks for a 2:3 poster.
const frameStyles = (frame: React.ReactElement) => {
  const markup = renderToStaticMarkup(<LoomarrProvider>{frame}</LoomarrProvider>);
  const tag = /<div[^>]*data-testid="frame"[^>]*>/.exec(markup)?.[0] ?? "";
  return declarationsOf(markup, tag);
};

describe("ArtworkFrame", () => {
  it("holds 16:9 by default", () => {
    const styles = frameStyles(<ArtworkFrame state="missing" testID="frame" />);
    expect(styles).toMatch(/aspect-ratio:1\.77/);
  });

  it("holds 2:3 for a poster", () => {
    const styles = frameStyles(<ArtworkFrame ratio="2:3" state="missing" testID="frame" />);
    expect(styles).toMatch(/aspect-ratio:0\.66/);
  });

  it("captions a poster with no artwork as the wide frame does", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ArtworkFrame ratio="2:3" state="missing" />
      </LoomarrProvider>,
    );
    expect(markup).toContain("No artwork");
  });

  it("shows the artwork it holds when ready", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ArtworkFrame ratio="2:3" state="ready">
          <Text textRole="body">Poster</Text>
        </ArtworkFrame>
      </LoomarrProvider>,
    );
    expect(markup).toContain("Poster");
    expect(markup).not.toContain("No artwork");
  });
});

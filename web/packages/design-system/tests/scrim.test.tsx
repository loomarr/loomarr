import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Scrim } from "../index";

// Read from the server's markup: jsdom's style parser drops a gradient with `calc` in it.
const styleOf = (element: React.ReactElement) =>
  Object.fromEntries(
    (/style="([^"]*)"/.exec(renderToStaticMarkup(element))?.[1] ?? "")
      .split(";")
      .map((rule) => [rule.slice(0, rule.indexOf(":")), rule.slice(rule.indexOf(":") + 1)]),
  );

describe("Scrim (web)", () => {
  it("feathers an ink gradient toward its edge", () => {
    const style = styleOf(<Scrim feather={64} from="top" />);
    expect(style["background-image"]).toBe(
      "linear-gradient(to top, rgba(11,12,14,0.95) calc(100% - 64px), transparent)",
    );
    expect(style["mask-image"]).toBeUndefined();
    expect(style.top).toBe("0");
  });

  it("reaches past its parent and fades out where it softens", () => {
    const style = styleOf(<Scrim feather={64} from="right" soften={32} strength={0.8} />);
    expect(style["background-image"]).toContain("to right, rgba(11,12,14,0.8)");
    expect(style["mask-image"]).toBe(
      "linear-gradient(transparent, black 32px, black calc(100% - 32px), transparent)",
    );
    expect([style.top, style.bottom]).toEqual(["-32px", "-32px"]);
  });
});

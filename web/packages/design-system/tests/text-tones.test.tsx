// @vitest-environment jsdom
import { getConfig } from "@tamagui/core";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { decadeTint, LoomarrProvider, semanticThemes, Text, type TextTone } from "../index";

afterEach(cleanup);

const modes = ["dark", "light"] as const;

const colourOf = (hex: string) => {
  const probe = document.createElement("span");
  probe.style.color = hex;
  return probe.style.color;
};

const drawn = (mode: (typeof modes)[number], tone: TextTone) => {
  render(
    <LoomarrProvider theme={mode}>
      <Text as="span" data-testid="numeral" textRole="mono" tone={tone}>
        7
      </Text>
    </LoomarrProvider>,
  );
  const colour = screen.getByTestId("numeral").style.color;
  cleanup();
  return colour;
};

describe("Text's decade tint tones", () => {
  it.each(modes)("draw the %s theme's own tint", (mode) => {
    for (const decade of [1950, 1960, 1970, 1980, 1990, 2000, undefined]) {
      const key = decade === undefined ? "neutral" : (String(decade) as "1950");
      expect(drawn(mode, decadeTint(decade))).toBe(colourOf(semanticThemes[mode].decadeTint[key]));
    }
  });

  it.each(modes)("leave every other tone's %s colour as it is", (mode) => {
    const themeKeys = {
      danger: "stateDanger",
      identOnair: "identLetterOnair",
      identSignal: "identLetterSignal",
      identSuggest: "identLetterSuggest",
      identTune: "identLetterTune",
      info: "stateInfo",
      inverse: "contentInverse",
      live: "stateLive",
      muted: "contentMuted",
      onDecade: "decadeInk",
      primary: "contentPrimary",
      secondary: "contentSecondary",
      signal: "actionPrimary",
      success: "stateSuccess",
      suggest: "accentSuggest",
      warning: "stateWarning",
    } as const;
    for (const [tone, key] of Object.entries(themeKeys)) {
      const entry = getConfig().themes[mode]?.[key] as unknown as { val: string };
      expect(drawn(mode, tone as TextTone), tone).toBe(colourOf(entry.val));
    }
  });
});

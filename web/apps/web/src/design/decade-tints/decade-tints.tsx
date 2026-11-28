import { Card, Stack, semanticThemes, Text } from "@loomarr/design-system";
import { contrast } from "@loomarr/design-system/web-theme";

// The Cross's decade tints (§2.1), each theme's set on that theme's grounds. Both themes are drawn
// whatever the toolbar says, and every hex and ratio is read from the tokens.

const rows = [
  ["1950", "1950s"],
  ["1960", "1960s"],
  ["1970", "1970s"],
  ["1980", "1980s"],
  ["1990", "1990s"],
  ["2000", "2000s"],
  ["2010", "2010s"],
  ["2020", "2020s"],
  ["neutral", "Breaks and no decade"],
] as const;

const modes = [
  ["dark", "Dark theme"],
  ["light", "Light theme"],
] as const;

const DecadeTints = () => (
  <Stack gap="section" padding="section">
    <Text as="h1" textRole="title">
      Decade tints
    </Text>
    {modes.map(([mode, title]) => {
      const theme = semanticThemes[mode];
      const ink = { color: theme.content.primary };

      return (
        <Stack aria-label={title} gap="control" key={mode} render="section">
          <Text as="h2" textRole="heading">
            {title}
          </Text>
          <Card backgroundColor={theme.surface.canvas} padding={16}>
            <Stack gap="control">
              {rows.map(([key, label]) => {
                const tint = theme.decadeTint[key];
                return (
                  <Stack align="center" direction="row" gap="control" key={key}>
                    <Text style={{ color: tint }} textRole="channelNumber">
                      {key === "neutral" ? "00" : key.slice(2)}
                    </Text>
                    <Stack flex={1} minWidth={0}>
                      <Text style={{ color: tint }} textRole="label">
                        {label}
                      </Text>
                      <Text style={ink} textRole="mono">
                        {tint}
                      </Text>
                    </Stack>
                    <Text style={ink} textRole="mono">
                      {`${contrast(tint, theme.surface.canvas).toFixed(2)} on canvas`}
                    </Text>
                  </Stack>
                );
              })}
            </Stack>
          </Card>
        </Stack>
      );
    })}
  </Stack>
);

export { DecadeTints };

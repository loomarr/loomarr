import { Card, Grid, roleLetterSpacing, Stack, Surface, Text, typography } from "@loomarr/design-system";
import { webType } from "@loomarr/design-system/web-theme";

const TYPE_ROLES = ["display", "heading", "body", "label", "mono"] as const;

// Typography — the type system documenting itself (frontend-design §2.2, §5.1a).
//
// Like the palette, every value is READ from the generated tokens. The scale is small and
// deliberate; a component reaching for a size that is not on it (`text-[10.5px]`) is a smell,
// and this page is where that becomes obvious rather than invisible.

// The rule that matters most, and the one most easily forgotten — it lives as a comment on the
// tokens today, which is a place nobody looks while writing a component.
const MONO_RULE = "If it came from a machine, it's mono.";

const SIZE_USE: Record<string, string> = {
  xs: "Captions, badge text, timestamps",
  sm: "Secondary copy, table cells",
  base: "Body — the default",
  md: "Emphasised body, card titles",
  lg: "Section headings",
  xl: "Page headings",
  "2xl": "Display — the wizard, empty states",
};

const Typography = () => (
  <Stack gap="screen" padding="section">
    <Stack gap="1" maxWidth={672} render="header">
      <Text as="h1" textRole="title">
        Typography
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        Geist and Geist Mono, self-hosted (§2.2) — no webfont CDN, because the app must work on a LAN with no
        internet. Sizes come from the generated tokens; the samples below render at their real values.
      </Text>
    </Stack>

    <Stack gap="control" render="section">
      <Text as="h2" textRole="heading">
        The two families
      </Text>
      <Grid columns={{ base: 1, sm: 2 }} gap="4">
        <Card gap="inline" padding={16}>
          <Text textRole="mono" tone="muted">
            Sans
          </Text>
          <Text style={{ fontFamily: webType.fonts.sans }} textRole="headline">
            Saturday Morning Cartoons
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Everything a person wrote or would say aloud: titles, descriptions, prose, buttons.
          </Text>
        </Card>
        <Card gap="inline" padding={16}>
          <Text textRole="mono" tone="muted">
            Mono
          </Text>
          <Text style={{ fontFamily: webType.fonts.mono }} textRole="headline">
            CH 42 · 21:30–22:00
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            <Text as="strong" strong textRole="compact" tone="primary">
              {MONO_RULE}
            </Text>{" "}
            Channel numbers, clock times, durations, ids, state badges — anything the system emitted rather
            than authored.
          </Text>
        </Card>
      </Grid>
    </Stack>

    <Surface backgroundColor="transparent" borderWidth={0} gap="control" padding={0}>
      <Text as="h2" density="pointer" strong textRole="heading">
        The five roles
      </Text>
      <Text density="pointer" textRole="body" tone="muted">
        Display, Heading, Body, Label and Mono, sized per density. Mono is for channel numbers and clocks
        only. Size / weight, read from the tokens.
      </Text>
      {TYPE_ROLES.map((role) => (
        <Surface
          key={role}
          backgroundColor="transparent"
          borderWidth={0}
          gap="control"
          padding={0}
          paddingVertical={12}
        >
          <Text density="pointer" textRole={role}>
            {role === "mono" ? "CH 42 · 21:30" : "Channels play what's ready"}
          </Text>
          <Text density="pointer" textRole="mono" tone="muted">
            {`${role}: ${(["pointer", "touch", "tv"] as const)
              .map((d) => `${d} ${typography[d][role].size}/${typography[d][role].weight}`)
              .join(" · ")}`}
          </Text>
        </Surface>
      ))}
    </Surface>

    <Stack gap="control" render="section">
      <Text as="h2" textRole="heading">
        Scale
      </Text>
      <Stack>
        {Object.entries(webType.size).map(([name, px], row) => (
          <Stack
            align="baseline"
            borderColor="borderDecorative"
            borderTopWidth={row === 0 ? 0 : 1}
            direction="row"
            gap="4"
            key={name}
            paddingVertical={12}
          >
            <Text flexShrink={0} textRole="mono" tone="muted" width={48}>
              {name}
            </Text>
            <Text fontVariant={["tabular-nums"]} flexShrink={0} textRole="mono" tone="muted" width={48}>
              {`${px}px`}
            </Text>
            <Text
              flex={1}
              minWidth={0}
              numberOfLines={1}
              style={{ fontSize: px, lineHeight: "inherit" }}
              textRole="body"
            >
              Channels play what's ready
            </Text>
            <Stack data-from-sm="" flexShrink={0}>
              <Text textRole="hint" tone="muted">
                {SIZE_USE[name]}
              </Text>
            </Stack>
          </Stack>
        ))}
      </Stack>
      <Stack maxWidth={672}>
        <Text as="p" textRole="compact" tone="secondary">
          Seven sizes, and nothing between them. A component reaching for an off-scale value is usually
          solving a spacing problem with type — the scale is short on purpose so that shows up as a decision
          rather than a drift.
        </Text>
      </Stack>
    </Stack>

    <Stack gap="control" render="section">
      <Text as="h2" textRole="heading">
        Leading
      </Text>
      <Grid columns={{ base: 1, sm: 2 }} gap="4">
        {Object.entries(webType.leading).map(([name, value]) => (
          <Card gap="inline" key={name} padding={16}>
            <Text textRole="mono" tone="muted">{`${name} · ${value}`}</Text>
            <Text as="p" style={{ lineHeight: value }} textRole="body">
              A channel is a wall clock, not a playlist that starts when someone watches. Tune in forty
              minutes into an hour-long film and you land forty minutes in.
            </Text>
          </Card>
        ))}
      </Grid>
    </Stack>
  </Stack>
);

// The display roles: type the app draws once each, kept at its own size rather than rounded onto the
// five roles above (#2147). Each sample renders through `Text` at the role's real values, and its
// caption reads size / line height / weight per density and the letter spacing from the tokens.
const DISPLAY_ROLES = [
  { role: "monogram", sample: "LM", use: "A channel's monogram on its generated art" },
  { role: "numeral", sample: "42", use: "The channel number in the info header" },
  { role: "pairCode", sample: "4F7-Q2K", use: "The code typed to pair a device" },
  { role: "microBadge", sample: "00:30", use: "A timecode or duration over a thumbnail" },
  { role: "footnote", sample: "12 of 40 channels · guide has the full week", use: "A line of fine print" },
  { role: "stationName", sample: "Saturday Cartoons", use: "The tuner's channel name, fluid on the web" },
  { role: "stationTag", sample: "CH 7", use: "The tuner's channel tag, fluid on the web" },
] as const;

const measure = (value: { lineHeight: number; size: number; weight: string }) =>
  `${value.size}/${value.lineHeight}/${value.weight}`;

const DisplayRoles = () => (
  <Stack gap="control" maxWidth={672} padding="section">
    <Text as="h1" textRole="title">
      Display roles
    </Text>
    {DISPLAY_ROLES.map(({ role, sample, use }) => (
      <Surface
        key={role}
        backgroundColor="transparent"
        borderWidth={0}
        gap="inline"
        padding={0}
        paddingVertical={12}
      >
        <Text as="p" textRole={role} tone={role === "stationTag" ? "signal" : undefined}>
          {sample}
        </Text>
        <Text as="p" textRole="hint" tone="secondary">
          {use}
        </Text>
        <Text as="p" textRole="mono" tone="muted">
          {`${role}: ${(["pointer", "touch", "tv"] as const)
            .map((d) => `${d} ${measure(typography[d][role])}`)
            .join(
              " · ",
            )}${role in roleLetterSpacing ? ` · tracking ${roleLetterSpacing[role as keyof typeof roleLetterSpacing]}em` : ""}`}
        </Text>
      </Surface>
    ))}
  </Stack>
);

export { DisplayRoles, Typography };

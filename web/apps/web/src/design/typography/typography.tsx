import { Stack, Surface, Text, typography } from "@loomarr/design-system";
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
  <Stack className="p-6" gap="screen">
    <Stack gap="1" render="header">
      <h1 className="text-xl font-semibold">Typography</h1>
      <p className="text-muted-foreground max-w-2xl text-sm">
        Geist and Geist Mono, self-hosted (§2.2) — no webfont CDN, because the app must work on a LAN with no
        internet. Sizes come from the generated tokens; the samples below render at their real values.
      </p>
    </Stack>

    <Stack gap="control" render="section">
      <h2 className="text-base font-semibold">The two families</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <Stack className="border-border rounded-md border p-4" gap="inline">
          <div className="text-static-400 font-mono text-[11px]">Sans</div>
          <div className="text-lg" style={{ fontFamily: webType.fonts.sans }}>
            Saturday Morning Cartoons
          </div>
          <p className="text-muted-foreground text-sm">
            Everything a person wrote or would say aloud: titles, descriptions, prose, buttons.
          </p>
        </Stack>
        <Stack className="border-border rounded-md border p-4" gap="inline">
          <div className="text-static-400 font-mono text-[11px]">Mono</div>
          <div className="font-mono text-lg" style={{ fontFamily: webType.fonts.mono }}>
            CH 42 · 21:30–22:00
          </div>
          <p className="text-muted-foreground text-sm">
            <strong className="text-foreground">{MONO_RULE}</strong> Channel numbers, clock times, durations,
            ids, state badges — anything the system emitted rather than authored.
          </p>
        </Stack>
      </div>
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
      <h2 className="text-base font-semibold">Scale</h2>
      <Stack className="divide-border divide-y">
        {Object.entries(webType.size).map(([name, px]) => (
          <Stack align="baseline" className="py-3" direction="row" gap="4" key={name}>
            <span className="text-static-400 w-12 shrink-0 font-mono text-[11px]">{name}</span>
            <span className="text-static-400 w-12 shrink-0 font-mono text-[11px] tabular-nums">
              {`${px}px`}
            </span>
            <span style={{ fontSize: px }} className="min-w-0 flex-1 truncate">
              Channels play what's ready
            </span>
            <span className="text-muted-foreground hidden shrink-0 text-xs sm:block">{SIZE_USE[name]}</span>
          </Stack>
        ))}
      </Stack>
      <p className="text-muted-foreground max-w-2xl text-sm">
        Seven sizes, and nothing between them. A component reaching for an off-scale value is usually solving
        a spacing problem with type — the scale is short on purpose so that shows up as a decision rather than
        a drift.
      </p>
    </Stack>

    <Stack gap="control" render="section">
      <h2 className="text-base font-semibold">Leading</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        {Object.entries(webType.leading).map(([name, value]) => (
          <Stack className="border-border rounded-md border p-4" gap="inline" key={name}>
            <div className="text-static-400 font-mono text-[11px]">{`${name} · ${value}`}</div>
            <p style={{ lineHeight: value }} className="text-sm">
              A channel is a wall clock, not a playlist that starts when someone watches. Tune in forty
              minutes into an hour-long film and you land forty minutes in.
            </p>
          </Stack>
        ))}
      </div>
    </Stack>
  </Stack>
);

export { Typography };

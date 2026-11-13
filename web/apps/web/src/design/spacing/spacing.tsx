import { Stack, Surface, spaceSteps, Text } from "@loomarr/design-system";
import { webMotion, webRadius, webRowHeight, webSpace } from "@loomarr/design-system/web-theme";

// Spacing, radius and motion — the geometry half of the system (§2.3, §2.4, §5.1a).
//
// Read from the generated tokens like every other Design page. Space is a 4px grid: the values
// are not arbitrary, they are multiples, and seeing them stacked is what makes an off-grid
// `gap-[7px]` obviously wrong rather than merely unusual. The half steps the app already uses sit
// in the same list at their own size, and the copy names them as the recorded exceptions they are.

const steps = Object.entries(spaceSteps).sort(([, a], [, b]) => a - b);
const halfSteps = Object.keys(spaceSteps).filter((step) => !(step in webSpace));

// A readout column: the mono label held at a fixed width so the rows line up.
const Column = ({ children, width }: { children: string; width?: number }) => (
  <Stack flexShrink={0} width={width}>
    <Text textRole="mono">{children}</Text>
  </Stack>
);

const Spacing = () => (
  <Stack gap="8" padding={24}>
    <Stack gap="1" render="header">
      <Text as="h1" textRole="title">
        Spacing, radius & motion
      </Text>
      <Stack maxWidth={672}>
        <Text as="p" textRole="compact" tone="secondary">
          A 4px grid. Every step is a multiple of four, which is what makes the interface feel aligned without
          anyone measuring — and what makes an off-grid value stand out. The half steps (
          {halfSteps.join(", ")}) are the exception: tight spacing inside a control or a chip, kept at its own
          size so a layout that uses one does not move when it adopts the scale.
        </Text>
      </Stack>
    </Stack>

    <Stack gap="3" render="section">
      <Text as="h2" textRole="heading">
        Space
      </Text>
      <Stack gap="2">
        {steps.map(([step, px]) => (
          <Stack align="center" direction="row" gap="4" key={step}>
            <Column width={32}>{step}</Column>
            <Column width={48}>{`${px}px`}</Column>
            <Stack
              borderRadius={webRadius.sm}
              className="bg-signal-tint-30"
              flexShrink={0}
              height={16}
              width={px}
            />
          </Stack>
        ))}
      </Stack>
    </Stack>

    <Stack gap="3" render="section">
      <Text as="h2" textRole="heading">
        Radius
      </Text>
      <Stack direction="row" gap="4" wrap>
        {Object.entries(webRadius).map(([name, px]) => (
          <Stack align="center" flexShrink={0} gap="2" key={name}>
            <Surface borderRadius={px} flexShrink={0} height={64} level="raised" width={64} />
            <Column>{`${name} · ${px}px`}</Column>
          </Stack>
        ))}
      </Stack>
    </Stack>

    <Stack gap="3" render="section">
      <Text as="h2" textRole="heading">
        Density
      </Text>
      <Stack gap="2">
        <Surface
          alignItems="center"
          borderRadius={webRadius.md}
          flexDirection="row"
          flexShrink={0}
          height={webRowHeight.compact}
          justifyContent="space-between"
          paddingHorizontal={12}
        >
          <Text textRole="body">A table row at compact density</Text>
          <Column>{`${webRowHeight.compact}px`}</Column>
        </Surface>
        <Stack maxWidth={672}>
          <Text as="p" textRole="compact" tone="secondary">
            Tables use the compact row height. A household's library runs to thousands of titles, so rows are
            sized to fit a screenful rather than to feel spacious.
          </Text>
        </Stack>
      </Stack>
    </Stack>

    <Stack gap="3" render="section">
      <Text as="h2" textRole="heading">
        Motion
      </Text>
      <Stack gap="2">
        {Object.entries(webMotion.duration).map(([name, value]) => (
          <Stack align="center" direction="row" gap="4" key={name}>
            <Column width={56}>{name}</Column>
            <Column width={64}>{value}</Column>
            <Text textRole="compact" tone="secondary">
              {name === "fast" ? "Hover, focus, small state flips" : "Panels, disclosure, page transitions"}
            </Text>
          </Stack>
        ))}
        <Stack align="center" direction="row" gap="4">
          <Column width={56}>ease</Column>
          <Column>{webMotion.ease}</Column>
        </Stack>
        <Stack maxWidth={672}>
          <Text as="p" textRole="compact" tone="secondary">
            One ease-out curve everywhere. Motion is short and decelerating — it should confirm that something
            happened, never make anyone wait for it. Under{" "}
            <Text as="code" textRole="mono">
              prefers-reduced-motion
            </Text>{" "}
            these collapse to near zero (§2.4), which the visual suite relies on to snapshot
            deterministically.
          </Text>
        </Stack>
      </Stack>
    </Stack>
  </Stack>
);

export { Spacing };

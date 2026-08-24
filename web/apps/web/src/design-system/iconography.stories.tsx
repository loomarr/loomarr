import { Icon, iconography, icons, semanticColors } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { FoundationsStoryShell } from "./foundation-story-shell";

const IconographyGallery = () => (
  <FoundationsStoryShell>
    <div style={{ display: "grid", gap: 40, margin: "0 auto", maxWidth: 1120 }}>
      <header style={{ display: "grid", gap: 8 }}>
        <h1 style={{ fontSize: 36, letterSpacing: "-0.02em", margin: 0 }}>Product iconography</h1>
        <p style={{ color: semanticColors.content.secondary, margin: 0, maxWidth: 760 }}>
          Lucide outlines at a {iconography.strokeWidth}px stroke, exposed through Loomarr names. Icons
          reinforce a visible label; icon-only controls require an accessible label and focus treatment.
        </p>
      </header>

      <section style={{ display: "grid", gap: 16 }}>
        <h2 style={{ fontSize: 16, margin: 0 }}>Approved glyph vocabulary</h2>
        <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(6, minmax(120px, 1fr))" }}>
          {Object.entries(icons).map(([name, glyph]) => (
            <div
              key={name}
              style={{
                alignItems: "center",
                background: semanticColors.surface.raised,
                border: `1px solid ${semanticColors.surface.elevated}`,
                borderRadius: 12,
                display: "grid",
                gap: 10,
                minHeight: 92,
                padding: 12,
              }}
            >
              <Icon decorative glyph={glyph} size="control" />
              <span
                style={{ color: semanticColors.content.secondary, fontFamily: "monospace", fontSize: 12 }}
              >
                {name}
              </span>
            </div>
          ))}
        </div>
      </section>

      <section style={{ display: "grid", gap: 16 }}>
        <h2 style={{ fontSize: 16, margin: 0 }}>Named sizes</h2>
        <div style={{ alignItems: "end", display: "flex", gap: 32 }}>
          {Object.entries(iconography.size).map(([name, pixels]) => (
            <div key={name} style={{ alignItems: "center", display: "grid", gap: 8, justifyItems: "center" }}>
              <Icon
                decorative
                glyph={icons.play}
                size={name as keyof typeof iconography.size}
                tone="primary"
              />
              <span style={{ color: semanticColors.content.muted, fontFamily: "monospace", fontSize: 12 }}>
                {name} · {pixels}px
              </span>
            </div>
          ))}
        </div>
      </section>

      <section style={{ display: "grid", gap: 16 }}>
        <h2 style={{ fontSize: 16, margin: 0 }}>Semantic treatments</h2>
        <div style={{ alignItems: "center", display: "flex", gap: 28 }}>
          <Icon accessibilityLabel="Play" glyph={icons.play} size="control" tone="primary" />
          <Icon accessibilityLabel="Information" glyph={icons.info} size="control" tone="secondary" />
          <Icon accessibilityLabel="Success" glyph={icons.success} size="control" tone="success" />
          <Icon accessibilityLabel="Warning" glyph={icons.warning} size="control" tone="warning" />
          <Icon accessibilityLabel="Unavailable" glyph={icons.close} size="control" tone="danger" />
          <Icon accessibilityLabel="Disabled" glyph={icons.settings} size="control" tone="disabled" />
        </div>
      </section>
    </div>
  </FoundationsStoryShell>
);

const meta = {
  title: "Loomarr Foundations/Iconography",
  component: IconographyGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof IconographyGallery>;

type Story = StoryObj<typeof meta>;
const Vocabulary: Story = {};

export default meta;
export { Vocabulary };

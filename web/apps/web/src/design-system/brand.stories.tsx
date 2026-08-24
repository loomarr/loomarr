import {
  BrandLockup,
  BrandMark,
  BrandWordmark,
  LoomarrProvider,
  semanticColors,
  semanticThemes,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";

import { FoundationsStoryShell } from "./foundation-story-shell";

const Section = ({ children, title }: { children: ReactNode; title: string }) => (
  <section style={{ display: "grid", gap: 16 }}>
    <h2 style={{ fontSize: 16, letterSpacing: "0.04em", margin: 0 }}>{title}</h2>
    {children}
  </section>
);

const ThemePreview = ({ mode }: { mode: "dark" | "light" }) => (
  <LoomarrProvider theme={mode}>
    <div
      style={{
        background: semanticThemes[mode].surface.canvas,
        color: semanticThemes[mode].content.primary,
        display: "grid",
        gap: 32,
        minHeight: 260,
        padding: 32,
        placeItems: "center",
      }}
    >
      <BrandLockup orientation="stacked" showTagline size="large" />
      <span style={{ color: semanticThemes[mode].content.secondary, fontFamily: "monospace", fontSize: 12 }}>
        {mode.toUpperCase()}
      </span>
    </div>
  </LoomarrProvider>
);

const BrandGallery = () => (
  <FoundationsStoryShell>
    <div style={{ display: "grid", gap: 40, margin: "0 auto", maxWidth: 1120 }}>
      <header style={{ display: "grid", gap: 8 }}>
        <h1 style={{ fontSize: 36, letterSpacing: "-0.02em", margin: 0 }}>Loomarr identity</h1>
        <p style={{ color: semanticColors.content.secondary, margin: 0, maxWidth: 720 }}>
          Loomarr's original test-card chroma bar, Geist wordmark, and explicit reductions for every surface.
          Refinement starts from the shipping identity; platform assets are generated from these forms, not
          alternate logos.
        </p>
      </header>

      <Section title="Primary lockups">
        <div style={{ display: "flex", flexWrap: "wrap", gap: 40 }}>
          <BrandLockup orientation="horizontal" showTagline size="large" />
          <BrandLockup orientation="stacked" showTagline size="large" />
        </div>
      </Section>

      <Section title="Light and dark themes">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(280px, 1fr))" }}>
          <ThemePreview mode="dark" />
          <ThemePreview mode="light" />
        </div>
      </Section>

      <Section title="One-color treatments">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(280px, 1fr))" }}>
          <LoomarrProvider theme="dark">
            <div
              style={{
                background: semanticThemes.dark.surface.canvas,
                display: "grid",
                minHeight: 160,
                placeItems: "center",
              }}
            >
              <BrandLockup tone="monochrome" />
            </div>
          </LoomarrProvider>
          <LoomarrProvider theme="light">
            <div
              style={{
                background: semanticThemes.light.surface.canvas,
                display: "grid",
                minHeight: 160,
                placeItems: "center",
              }}
            >
              <BrandLockup tone="monochrome" />
            </div>
          </LoomarrProvider>
        </div>
      </Section>

      <Section title="Small-size reductions">
        <div style={{ alignItems: "end", display: "flex", gap: 28 }}>
          {[16, 24, 32, 48, 72].map((size) => (
            <div key={size} style={{ alignItems: "center", display: "grid", gap: 8, justifyItems: "center" }}>
              <BrandMark size={size} />
              <span style={{ color: semanticColors.content.muted, fontFamily: "monospace", fontSize: 12 }}>
                {size}px
              </span>
            </div>
          ))}
          <BrandWordmark size="small" />
        </div>
      </Section>

      <Section title="Generated platform assets">
        <div
          style={{
            display: "grid",
            gap: 20,
            gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
          }}
        >
          {[
            {
              alt: "Loomarr mobile launcher icon",
              label: "MOBILE / WEB APP ICON",
              src: "/generated-brand/mobile/icon.png",
            },
            {
              alt: "Loomarr Play store icon",
              label: "PLAY STORE ICON",
              src: "/generated-brand/store/play-icon-512x512.png",
            },
            {
              alt: "Loomarr TV launcher banner",
              label: "TV LAUNCHER BANNER",
              src: "/generated-brand/tv/tv-banner.png",
            },
          ].map((asset) => (
            <figure
              key={asset.label}
              style={{
                background: semanticThemes.dark.surface.raised,
                border: `1px solid ${semanticThemes.dark.border.decorative}`,
                borderRadius: 16,
                display: "grid",
                gap: 12,
                margin: 0,
                overflow: "hidden",
                padding: 16,
              }}
            >
              <img
                alt={asset.alt}
                src={asset.src}
                style={{ aspectRatio: "16 / 9", objectFit: "contain", width: "100%" }}
              />
              <figcaption
                style={{
                  color: semanticThemes.dark.content.secondary,
                  fontFamily: "monospace",
                  fontSize: 12,
                }}
              >
                {asset.label}
              </figcaption>
            </figure>
          ))}
        </div>
        <p style={{ color: semanticColors.content.secondary, fontSize: 13, margin: 0 }}>
          These committed files are generated from the same contract as the in-product mark. The drift gate
          rejects a hand-edited favicon, launcher icon, TV banner, or store derivative.
        </p>
      </Section>

      <Section title="Clear space">
        <div
          style={{
            border: `1px dashed ${semanticColors.action.focus}`,
            display: "inline-grid",
            justifySelf: "start",
            padding: 24,
          }}
        >
          <BrandLockup />
        </div>
        <p style={{ color: semanticColors.content.secondary, fontSize: 13, margin: 0 }}>
          Keep at least half the chroma strip height clear on every side. Never reorder or recolor its seven
          segments, compress the wordmark, add effects, or place the full-color mark on a busy image.
        </p>
      </Section>
    </div>
  </FoundationsStoryShell>
);

const meta = {
  title: "Loomarr Foundations/Brand",
  component: BrandGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof BrandGallery>;

type Story = StoryObj<typeof meta>;
const IdentitySystem: Story = {};

export default meta;
export { IdentitySystem };

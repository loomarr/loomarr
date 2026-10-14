# Claude Design prompt — Composites & lineage (Loomarr V45)

Design the UI for **composite filler clips and their split lineage** in Loomarr — a self-hosted app
that turns a natural-language channel intent into a live retro-TV channel, complete with era-matched
commercial breaks. This surface lives in the **Filler** section (a 3-tab area: Catalog · Incoming ·
Sources). Design for **desktop and mobile**.

## Aesthetic — match this exactly, do not invent a new style

**A modern, dark broadcast console** — calm surfaces, precise data, mono-set numbers — with retro-TV
warmth used as *seasoning, not sauce*. It should feel like the master-control room of a TV station:
professional first, nostalgic in the margins. Dark-only.

**The "Test Card" palette** (derived from SMPTE color bars, tuned for WCAG AA on dark):

| Token | Hex | Use |
| --- | --- | --- |
| `static-950` | `#0B0C0E` | app background |
| `static-900` | `#131519` | cards, panels |
| `static-800` | `#1B1E24` | nested / hover surfaces |
| `static-700` | `#2A2E37` | hairlines, dividers |
| `static-400` | `#8B93A3` | secondary text |
| `static-100` | `#E7EAF0` | body text |
| `signal` (amber) | `#FFB020` | **brand & primary actions**, focus ring, active nav |
| `onair` (red) | `#E5484D` | live / destructive |
| `suggest` (magenta) | `#D6409F` | **the AI color** — AI-derived tags, generation |
| `tune` (cyan) | `#4CC9E8` | links, informational, "in progress" |
| `lock` (green) | `#3DD68C` | success, "signal locked", high confidence |
| `caution` (yellow) | `#F5D90A` | warnings, drift, low confidence |

Accent backgrounds are **alpha-wash tints** (`color-mix(accent, N% , transparent)`), N ∈ {8,12,15}
for text-bearing chips. Badges are **small, mono, uppercase**. Type is a clean grotesk for UI, mono
for numbers/timecodes/channel data. Corners are subtle (rounded-sm to rounded-md), borders are
1px hairlines in `static-700`.

## The domain — what a "composite" is

Operators pull filler from the Internet Archive. Some downloads are **single ads**; others are
**composites** — a whole recorded commercial break in one file, e.g.:

> **"KCPQ/Fox commercials, 5/28/1996"** — 16 minutes 11 seconds, ~41 individual ads
> (A&W root beer, US Navy, Dove, KFC, Snapple, a Paramount movie trailer, Reach toothbrush,
> Healthy Choice cereal, a Nestlé Butterfinger Christmas ad, a Dodge Memorial Day sale…).

A composite is **detected on import** and is **NOT airable** — it's a container, not a commercial.
Loomarr splits it into **segments** (the individual ads), which ARE airable. Each segment keeps a
**lineage link back to its parent break**. The parent is kept forever (for provenance and re-splitting).

Rich metadata Loomarr extracts (design for showing it, gracefully degrading when absent):
- Per composite: **network** (Fox), **station** (KCPQ), **market** (Seattle), **air date** (1996-05-28)
- Per segment: **name** (A&W root beer), **duration** (0:30), **era** (1996), **brand** (A&W),
  **audience** (general/kids/family/late_night), **category** (fast_food/cars/cereal…), and a
  **confidence** score (0–100) for how sure the auto-split/tagging was.

## Design these three surfaces

### 1. Composite in the Catalog — an **expandable group, inline**
The catalog is a grid/list of airable clips. A composite appears as a **distinct row that expands to
reveal its segments inline** — you see "the break" and can drill into "its ads." It must read clearly
as *a container, not an airable clip* (a "COMPOSITE · not airable" badge, broadcast-context shown:
`FOX · SEATTLE · 5/28/1996`). Collapsed: the break as one row with segment count + total runtime.
Expanded: its segments listed/gridded beneath, each a normal clip card.

### 2. Lineage — the **delightful** view (pick the best, effort is not a constraint)
The hero moment. A composite should feel like a **navigable map of its break**, not a dead 16-minute
row. The strongest idea: a **time-scaled filmstrip** where each block's WIDTH is proportional to that
ad's duration (a 45s spot is visibly wider than a 3s sting) — so a good split vs a bad one is legible
at a glance — and each block is **clickable to jump to that ad's clip**, labeled with its brand and
timecode, tinted by **confidence** (green `lock` = high, amber `signal` = medium, yellow `caution` =
low). Segments show "From: KCPQ/Fox 5/28/1996 →" back to the parent. Show the reverse too (the break
listing its ~41 segments). Make the segment↔break relationship feel continuous and explorable.
Delight us — animate the expand, let a hover on a filmstrip block preview the ad, whatever elevates it.

### 3. Split-review flow — the confirm step
Before segments air, an operator reviews the proposed cuts. The mental model is **"file these ads
under this break"** (the break is KEPT), NOT "destroy the compilation." Show the same filmstrip with
editable cuts (rename / merge-adjacent / drop a sliver), each segment's proposed tags, and a clear
"Confirm — file N ads under this break" action. Low-confidence segments should visibly want review;
high-confidence ones should feel safe to accept.

## Deliverable
Desktop + mobile. Show: the collapsed composite row, the expanded state with the filmstrip lineage
view, a segment clip card with its lineage link, and the split-review screen. Use the exact palette
and the broadcast-console tone above. Favor clarity and precise data density over decoration —
this is a control room, not a consumer app.

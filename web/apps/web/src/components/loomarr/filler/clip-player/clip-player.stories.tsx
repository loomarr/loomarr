import type { ClipDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Button } from "@/components/ui";
import { TINY_MP4 } from "@/test/video-fixture";
import { ClipPlayer } from "./clip-player";

// ClipPlayer — watch one catalog clip in a modal (V39).
//
// Deliberately thin: everything about PLAYING is the `VideoPlayer` primitive's job, and everything
// here is about the dialog and about clips. That split is what lets the channel-watch surface the
// mock sketches reuse the same player for a live stream without inheriting `ClipDTO`.
//
// ⚠ **THIS STORY SHOWS THE TRIGGER, NOT AN OPEN PLAYER**, for the reason `Dialog`'s own story
// records at length: Radix portals content to `document.body`, while the visual suite snapshots
// `#storybook-root` and waits for a visible child of it — so a forced-open dialog hangs that wait
// until the test times out. It shipped that way once already.
//
// The player's own surface IS covered, at `UI/VideoPlayer`, which renders unportalled. Rebuilding
// its chrome here with hand-written markup would snapshot a lookalike rather than the component,
// which is worse than not snapshotting it.
const meta = {
  title: "Filler/ClipPlayer",
  component: ClipPlayer,
} satisfies Meta<typeof ClipPlayer>;

type Story = StoryObj<typeof meta>;

// ⚠ `hash` carries the inline fixture rather than a real content hash, and that is what makes
// this story work offline: `clipMediaURL` passes a `data:` value through UNCHANGED (and only
// `data:` — never `http(s):`, which would turn a clip's identity into a request to an arbitrary
// origin). The visual suite runs against `storybook-static` with no server, so a real hash would
// 404.
const clip: ClipDTO = {
  name: "CTV-LifewithBonnie.mkv — CTV Life with Bonnie",
  kind: "commercial",
  durationMs: 30_000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  hash: TINY_MP4,
  tunarrProgramId: "story-clip",
};

// A real component rather than an inline `render` body: hooks belong in one, and the repo's style
// rule wants an arrow-function expression rather than a named `function` declaration.
const OpensOnClick = () => {
  const [playing, setPlaying] = useState<ClipDTO | null>(null);
  return (
    <>
      <Button onClick={() => setPlaying(clip)}>Play clip</Button>
      <ClipPlayer clip={playing} onClose={() => setPlaying(null)} />
    </>
  );
};

// ⚠ `args` is required by the typing even though `render` ignores them: `ClipPlayer`'s props are
// non-optional, so Storybook insists the story can supply them. The closed state is the honest
// default here anyway — see the portal note above.
const Default: Story = {
  args: { clip: null, onClose: () => {} },
  render: () => <OpensOnClick />,
};

export default meta;
export { Default };

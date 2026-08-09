import type { ImageDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { Image } from "./image";

// THE image primitive (§22, V52). Every surface that shows something the image service stores
// goes through this; none hand-writes an `<img>` against `/v1/images`.
//
// ⚠ **Every URL here is an inline `data:` URI, never a remote one.** A story that fetches from
// an origin races the visual snapshot — the shot lands before or after the load depending on the
// network, and the suite goes flaky in a way that reads as a component bug. This repo already
// has that lesson written down; these are 1×1 PNGs scaled up by the layout.

// A 1×1 amber PNG and a 1×1 slate one — enough to prove a rendition was chosen and painted.
const AMBER =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQzwAEjP8ZAAoDAv8T7QhZAAAAAElFTkSuQmCC";
const SLATE =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgYPj/HwAEAQH/7uJ9WQAAAABJRU5ErkJggg==";

// A real ThumbHash of a warm 2:3 image, so the placeholder path is genuinely exercised rather
// than skipped by an empty string.
const THUMBHASH = "1QcSHQRnh493V4dIh4eXh1h4kJUI";

const poster = (over: Partial<ImageDTO> = {}): ImageDTO => ({
  hash: "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503",
  role: "poster",
  width: 500,
  height: 750,
  placeholder: THUMBHASH,
  dominantHex: "#7a5230",
  animated: false,
  // Width descriptors across the poster ladder. The bytes are identical here; what the story
  // proves is that a `srcset` is emitted at all — there was none anywhere in the app before this.
  srcSetWebp: `${AMBER} 154w, ${AMBER} 342w, ${AMBER} 500w`,
  srcSetAvif: `${SLATE} 154w, ${SLATE} 342w, ${SLATE} 500w`,
  src: AMBER,
  ...over,
});

const meta = {
  title: "UI/Image",
  component: Image,
  decorators: [widthFrame(340)],
  args: { alt: "Channel poster", sizes: "320px" },
} satisfies Meta<typeof Image>;

type Story = StoryObj<typeof meta>;

// The ordinary case: both formats offered, the browser picks.
const Poster: Story = { args: { image: poster() } };

// ⚠ **The normal state of a freshly-adopted image, not an edge case.** AVIF is produced by a
// background job that runs at :20, so for up to an hour after an image lands there is no AVIF
// rendition — and `<picture>` handles it natively by taking WebP. Nothing waits, and no surface
// has to know whether the job has caught up.
const AwaitingAvif: Story = { args: { image: poster({ srcSetAvif: "" }) } };

// The LCP case. `priority` flips loading, fetch priority and decoding together — a half-applied
// priority is worse than none, because it reads as handled.
const Priority: Story = { args: { image: poster(), priority: true } };

// ⚠ An icon is whatever shape the operator uploaded, which is why the component takes real
// measured dimensions rather than deriving them from the role. A 2:3 assumption would letterbox
// this one.
const WideIcon: Story = {
  args: {
    image: poster({ role: "icon", width: 512, height: 168, dominantHex: "#2b4a5e" }),
    alt: "Channel logo",
    sizes: "320px",
  },
};

// The failure path, at its default: a flat block in the image's own dominant colour, holding the
// right aspect. ⚠ Deliberately NOT the blurred placeholder — a permanent blur is indistinguishable
// from a slow network, which is exactly how §22's accepted "uploads do not survive losing
// /data/images" would come to look like a bug that might still fix itself.
const Broken: Story = {
  args: { image: poster({ src: "data:image/png;base64,not-an-image", srcSetWebp: "", srcSetAvif: "" }) },
};

// A caller with a real designed empty state passes it. This is what a channel tile does — the
// monogram is a better answer than any colour block, because it still identifies the channel.
const BrokenWithDesignedFallback: Story = {
  args: {
    image: poster({ src: "data:image/png;base64,not-an-image", srcSetWebp: "", srcSetAvif: "" }),
    fallback: (
      <div className="flex aspect-[2/3] w-full items-center justify-center rounded-md bg-tune/20 font-semibold text-2xl text-tune">
        AH
      </div>
    ),
  },
};

// ⚠ No placeholder and no dominant colour — the state an image row is in between `Adopt` and the
// fetch job running. It must render as a deliberate empty box, not as nothing.
const NoPlaceholder: Story = {
  args: { image: poster({ placeholder: "", dominantHex: "" }) },
};

export default meta;
export { AwaitingAvif, Broken, BrokenWithDesignedFallback, NoPlaceholder, Poster, Priority, WideIcon };

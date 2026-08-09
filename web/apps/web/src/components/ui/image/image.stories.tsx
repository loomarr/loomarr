import type { ImageDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { Image } from "./image";

// THE image primitive (§22, V52). Every surface that shows something the image service stores
// goes through this; none hand-writes an `<img>` against `/v1/images`.
//
// ⚠ **These URLs are same-origin STATIC ASSETS, and they used to be `data:` URIs — which silently
// broke every story in this file (#210).** A base64 data URI always contains a comma, and a comma
// is `srcset`'s candidate separator, so every candidate was unloadable: the <img> rendered at
// naturalWidth 0 and each baseline captured the ThumbHash placeholder instead of an image. Green
// forever, proving nothing about the one attribute this primitive exists to emit.
//
// Remote URLs are still banned (they race the snapshot). These come from
// `.storybook/story-assets/` via `staticDirs`, so they are same-origin — no race — and comma-free.
//
// ⚠ **Each rung is a DIFFERENT COLOUR on purpose.** The baseline then shows WHICH candidate the
// browser chose, not merely that something painted: at `sizes="320px"` and DPR 1 (pinned by
// playwright.shared), 320 CSS px selects the 342w rung. A regression that collapsed the ladder to
// one rung, or that fell through to `src`, changes the colour and the diff is unmissable.
const POSTER_SRCSET = "/poster-154.webp 154w, /poster-342.webp 342w, /poster-500.webp 500w";
// AVIF is a single distinct colour across all three rungs: its job is to prove <picture> PREFERS
// AVIF when one exists, not to distinguish rungs (the WebP set already does that).
const POSTER_SRCSET_AVIF = "/poster-154.avif 154w, /poster-342.avif 342w, /poster-500.avif 500w";
// A further distinct colour. If a baseline ever shows THIS, no srcset candidate was used at all.
const POSTER_FALLBACK = "/poster-fallback.jpg";

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
  srcSetWebp: POSTER_SRCSET,
  // ⚠ Empty by DEFAULT, because that is the real state of a freshly-ingested image: AVIF is
  // job-produced, so most images do not have one yet. The backend gates the advertisement on the
  // rendition EXISTING — advertising an AVIF that 404s makes <picture> commit to a source it
  // cannot load and breaks the image outright, since its fallback chain is for format SUPPORT and
  // not availability. The `Poster` story opts in explicitly.
  srcSetAvif: "",
  src: POSTER_FALLBACK,
  ...over,
});

const meta = {
  title: "UI/Image",
  component: Image,
  decorators: [widthFrame(340)],
  args: { alt: "Channel poster", sizes: "320px" },
} satisfies Meta<typeof Image>;

type Story = StoryObj<typeof meta>;

// ⚠ **The DEFAULT is "no AVIF yet", because that is the ordinary state of an image.** AVIF is
// produced by a background job, so for up to an hour after an image lands there is no AVIF
// rendition — and forever, if an operator drops `avif` from `images.formats`. `<picture>` takes
// WebP and nothing waits. The baseline should show the 342w WebP colour: at `sizes="320px"` and
// DPR 1, 320 CSS px selects the 342w rung.
const AwaitingAvif: Story = { args: { image: poster() } };

// Both formats offered, and the browser prefers AVIF. The baseline should show the AVIF colour,
// NOT the WebP one — that difference is the whole assertion.
//
// ⚠ This only renders because the AVIF files genuinely exist. Advertising an AVIF that 404s does
// not degrade to WebP: `<picture>` commits to the source it selects by `type`, and its fallback
// chain is for format SUPPORT, not availability. That shipped once and broke every image in the
// app; the backend now gates the advertisement on the rendition existing.
const Poster: Story = { args: { image: poster({ srcSetAvif: POSTER_SRCSET_AVIF }) } };

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
  args: { image: poster({ src: "/does-not-exist.jpg", srcSetWebp: "", srcSetAvif: "" }) },
};

// A caller with a real designed empty state passes it. This is what a channel tile does — the
// monogram is a better answer than any colour block, because it still identifies the channel.
const BrokenWithDesignedFallback: Story = {
  args: {
    image: poster({ src: "/does-not-exist.jpg", srcSetWebp: "", srcSetAvif: "" }),
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

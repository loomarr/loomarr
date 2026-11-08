// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Real library art, when it is on this machine. ./real-art/ is gitignored (the repo is public, and the
// artwork and titles are someone else's): a manifest plus a few titles' clearlogo and fanart pulled
// from the maintainer's Emby. With the folder empty the mocks keep their invented titles and drawn
// stand-ins, so nothing else changes.

interface RealProgramme {
  line: string;
  synopsis: string;
  tags: string[];
  title: string;
}

interface RealChannel extends Partial<RealProgramme> {
  dominantHex?: string;
  fanart: string;
  logo: string;
  /** The logo's width over height, so the band can size it before the image loads. */
  ratio: number;
  title: string;
}

interface Manifest {
  channels: Record<string, RealChannel>;
  /** What follows CH 7's programme. */
  next?: RealProgramme;
  picture: string;
}

const files = import.meta.glob<string>("./real-art/*.{png,jpg}", {
  eager: true,
  import: "default",
  query: "?url",
});
const manifests = import.meta.glob<Manifest>("./real-art/manifest.json", { eager: true, import: "default" });
const manifest: Manifest | undefined = manifests["./real-art/manifest.json"];

const url = (name: string): string | undefined => files[`./real-art/${name}`];

interface Logo {
  alt: string;
  ratio: number;
  src: string;
}

/** A channel's real title, with its files resolved; undefined when it isn't on this machine. */
const realChannel = (number: string): (RealChannel & { fanartSrc: string; logoOf: Logo }) | undefined => {
  const entry = manifest?.channels[number];
  const fanartSrc = entry ? url(entry.fanart) : undefined;
  const logoSrc = entry ? url(entry.logo) : undefined;
  return entry && fanartSrc && logoSrc
    ? { ...entry, fanartSrc, logoOf: { alt: entry.title, ratio: entry.ratio, src: logoSrc } }
    : undefined;
};

const pictureSrc = manifest ? url(manifest.picture) : undefined;

/**
 * The picture: a real 16:9 still standing in for the video, contained with black bars exactly as the
 * video would be (a slim letterbox on a 16:10 laptop, a mid-screen strip on a portrait phone).
 */
const RealFrame = () => (
  <div
    aria-hidden
    style={{
      background: `#000 url("${pictureSrc ?? ""}") center/contain no-repeat`,
      inset: 0,
      position: "absolute",
    }}
  />
);

const realNext = manifest?.next;

export type { Logo, RealChannel, RealProgramme };
export { pictureSrc, RealFrame, realChannel, realNext };

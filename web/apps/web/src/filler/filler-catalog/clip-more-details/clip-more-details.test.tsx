import type { ClipDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import {
  ClipMoreDetails,
  displayAudioChannels,
  displayAudioRate,
  displayCodec,
  displayFrameRate,
} from "./clip-more-details";

const clip = (over: Partial<ClipDTO> = {}): ClipDTO => ({
  hash: "catalog-hash",
  name: "Local soda commercial",
  kind: "commercial",
  era: 1990,
  audience: "general",
  category: "drinks",
  durationMs: 30_000,
  source: "folder",
  playCount: 0,
  playsCounted: true,
  aiTagged: false,
  tagged: true,
  needsDetails: false,
  suggestedEra: 0,
  ...over,
});

describe("display helpers", () => {
  it("names known codecs and upper-cases the rest", () => {
    expect(displayCodec("h264")).toBe("H.264");
    expect(displayCodec("HEVC")).toBe("HEVC");
    expect(displayCodec("vp9")).toBe("VP9");
    expect(displayCodec(undefined)).toBeUndefined();
  });

  it("turns a rational cadence into frames per second", () => {
    expect(displayFrameRate("24000/1001")).toBe("23.98 fps");
    expect(displayFrameRate("30")).toBe("30 fps");
    expect(displayFrameRate("0/1")).toBeUndefined();
    expect(displayFrameRate("abc")).toBeUndefined();
  });

  it("describes audio channels and rate", () => {
    expect(displayAudioChannels(1)).toBe("mono");
    expect(displayAudioChannels(2)).toBe("stereo");
    expect(displayAudioChannels(6)).toBe("6 channels");
    expect(displayAudioChannels(0)).toBeUndefined();
    expect(displayAudioRate(48000)).toBe("48 kHz");
    expect(displayAudioRate(44100)).toBe("44.1 kHz");
    expect(displayAudioRate(undefined)).toBeUndefined();
  });
});

describe("ClipMoreDetails", () => {
  it("lists the file's video, audio and container facts", async () => {
    render(
      <ClipMoreDetails
        clip={clip({
          media: {
            width: 1920,
            height: 1080,
            frameRate: "24000/1001",
            container: "mp4",
            videoCodec: "h264",
            audioCodec: "aac",
            audioChannels: 2,
            audioRateHz: 48000,
            bytes: 9361105,
          },
        })}
      />,
    );
    await userEvent.click(screen.getByText("More about this clip"));

    expect(screen.getByText("1920×1080 · H.264 · 23.98 fps")).toBeVisible();
    expect(screen.getByText("AAC · stereo · 48 kHz")).toBeVisible();
    expect(screen.getByText("MP4 · 8.9 MB")).toBeVisible();
  });

  it("falls back to the stored resolution when the file was never probed", () => {
    render(<ClipMoreDetails clip={clip({ quality: "720p" })} />);

    expect(screen.getByText("Resolution")).toBeInTheDocument();
    expect(screen.getByText("720p")).toBeInTheDocument();
    expect(screen.queryByText("Video")).not.toBeInTheDocument();
  });

  it("shows language, licence and how each detail was found", () => {
    render(
      <ClipMoreDetails
        clip={clip({
          language: "none",
          license: "Public domain",
          enrichment: {
            state: "details_limited",
            facts: [
              { axis: "kind", evidence: "item_metadata" },
              { axis: "era", evidence: "operator" },
            ],
          },
        })}
      />,
    );

    expect(screen.getByText("No speech")).toBeInTheDocument();
    expect(screen.getByText("Public domain")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("Item details")).toBeInTheDocument();
    expect(screen.getByText("Year")).toBeInTheDocument();
    expect(screen.getByText("You set this")).toBeInTheDocument();
  });

  it("says plainly when nothing has been recorded", () => {
    render(<ClipMoreDetails clip={clip()} />);
    expect(screen.getByText("No additional details recorded yet.")).toBeInTheDocument();
  });
});

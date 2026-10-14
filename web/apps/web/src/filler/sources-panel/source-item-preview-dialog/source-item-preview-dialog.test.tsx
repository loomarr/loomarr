import { describe, expect, it } from "vitest";
import { providerEmbedURL } from "./source-item-preview-dialog";

describe("providerEmbedURL", () => {
  it("builds only provider-owned autoplay embeds", () => {
    expect(providerEmbedURL("archive", "https://archive.org/details/station_break_one")).toBe(
      "https://archive.org/embed/station_break_one?autoplay=1",
    );
    expect(providerEmbedURL("youtube", "https://www.youtube.com/watch?v=video-one")).toBe(
      "https://www.youtube-nocookie.com/embed/video-one?autoplay=1&playsinline=1",
    );
    expect(providerEmbedURL("youtube", "https://example.com/watch?v=video-one")).toBeUndefined();
    expect(providerEmbedURL("archive", "not a URL")).toBeUndefined();
  });
});

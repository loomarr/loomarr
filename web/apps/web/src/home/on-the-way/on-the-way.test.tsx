import { describe, expect, it } from "vitest";
import { titleRow } from "./on-the-way";

describe("On the way — a title's row (#1667)", () => {
  const ch = { id: "c1", name: "Late Sci-Fi", number: 7 };

  it("shows a downloading series' channel, episodes, progress and time left", () => {
    const row = titleRow({
      key: "tv:1",
      name: "A sci-fi anthology",
      mediaType: "series",
      state: "downloading",
      progress: 0.22,
      etaText: "about 40 min",
      episodesHave: 8,
      episodesWanted: 36,
      channels: [ch],
    });
    expect(row.sub).toBe("For Late Sci-Fi · 8 of 36 episodes");
    expect(row.progress).toEqual({ value: 22, label: "Downloading", eta: "about 40 min" });
  });

  it("marks a title the download client hasn't taken yet as queued", () => {
    const row = titleRow({
      key: "tv:2",
      name: "A cartoon",
      mediaType: "series",
      state: "requested",
      channels: [ch],
    });
    expect(row.sub).toBe("For Late Sci-Fi · waiting for a download slot");
    expect(row.progress).toEqual({ label: "Waiting to download", eta: "queued" });
  });
});

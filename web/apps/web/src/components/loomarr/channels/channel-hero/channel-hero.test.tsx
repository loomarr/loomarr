import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import { getChannelTimelineMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { ChannelHero } from "./channel-hero";

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};

const image = (src: string, role: string): ImageDTO => ({
  animated: false,
  dominantHex: "#222222",
  hash: src,
  height: 720,
  placeholder: "",
  role,
  src,
  srcSetAvif: "",
  srcSetWebp: `${src}.webp 1280w`,
  width: 1280,
});

const airing = (over: Partial<GuideAiring>): GuideAiring => ({
  kind: "program",
  scheduleBlockId: "b",
  startMs: Date.now() - 600_000,
  stopMs: Date.now() + 600_000,
  title: "Show",
  ...over,
});

const show = (airings: GuideAiring[]) => {
  server.use(getChannelTimelineMockHandler({ airings, serverNowMs: Date.now() }));
  return render(<ChannelHero channelId="ch-1" channel={{ name: "Field Notes", number: 7 }} />, { wrapper });
};

describe("ChannelHero (#2753)", () => {
  it("opens on the backdrop of what is on now", async () => {
    show([
      airing({ scheduleBlockId: "now", backdropImage: image("/img/now-backdrop", "backdrop") }),
      airing({
        scheduleBlockId: "next",
        startMs: Date.now() + 600_000,
        stopMs: Date.now() + 1_200_000,
        backdropImage: image("/img/next-backdrop", "backdrop"),
      }),
    ]);
    await waitFor(() => expect(document.querySelector("img")?.getAttribute("src")).toBe("/img/now-backdrop"));
  });

  it("falls to the still when the title has no backdrop", async () => {
    show([airing({ thumbImage: image("/img/still", "thumb") })]);
    await waitFor(() => expect(document.querySelector("img")?.getAttribute("src")).toBe("/img/still"));
  });

  it("is generated channel art, never an empty box, when no title has a picture", async () => {
    show([airing({})]);
    expect(await screen.findByText("FN")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
  });

  it("takes the backdrop's 16:9 shape up to 48rem, with no height cap to crop it", async () => {
    show([airing({ backdropImage: image("/img/now-backdrop", "backdrop") })]);
    await waitFor(() => expect(document.querySelector("img")).not.toBeNull());
    const hero = document.querySelector("[data-channel-hero]") as HTMLElement;
    expect(hero).toHaveStyle({ aspectRatio: `${16 / 9}`, maxWidth: "768px", width: "100%" });
    expect(hero.style.maxHeight).toBe("");
    expect(getComputedStyle(hero).maxHeight).not.toMatch(/px$/);
  });

  it("is generated channel art while the schedule is empty", async () => {
    show([]);
    expect(await screen.findByText("FN")).toBeInTheDocument();
  });
});

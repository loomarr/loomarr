import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { AiringArtwork } from "./on-now";

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

const channel = { name: "Field Notes", number: 7 };

describe("AiringArtwork (#2762)", () => {
  it("prefers the title's backdrop over the media server's still", () => {
    render(
      <AiringArtwork
        airing={{
          backdropImage: image("/img/backdrop", "backdrop"),
          thumbImage: image("/img/still", "thumb"),
        }}
        channel={channel}
        sizes="100vw"
      />,
    );
    expect(document.querySelector("img")?.getAttribute("src")).toBe("/img/backdrop");
  });

  it("uses the still when the airing has no backdrop", () => {
    render(
      <AiringArtwork airing={{ thumbImage: image("/img/still", "thumb") }} channel={channel} sizes="100vw" />,
    );
    expect(document.querySelector("img")?.getAttribute("src")).toBe("/img/still");
  });

  it("falls to generated channel art when a still URL fails, never a blank box", () => {
    render(<AiringArtwork airing={{ thumbUrl: "/img/gone" }} channel={channel} sizes="100vw" />);
    fireEvent.error(document.querySelector("img") as HTMLImageElement);
    expect(document.querySelector("img")).toBeNull();
    expect(screen.getByText("FN")).toBeInTheDocument();
  });

  it("falls to generated channel art when the backdrop fails to load", () => {
    render(
      <AiringArtwork
        airing={{ backdropImage: image("/img/backdrop", "backdrop") }}
        channel={channel}
        sizes="100vw"
      />,
    );
    fireEvent.error(document.querySelector("img") as HTMLImageElement);
    expect(screen.getByText("FN")).toBeInTheDocument();
  });
});

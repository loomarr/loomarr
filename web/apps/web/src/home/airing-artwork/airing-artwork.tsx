import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { Image } from "@/components/ui/image";
import type { AiringArtworkProps } from "./airing-artwork.type";

// AiringArtwork — the picture in a Home card's picture slot: the airing's image-service artwork,
// else its legacy thumbnail URL, else the code-drawn fallback. A card always has a picture, so a
// title with no artwork still fills the slot rather than leaving a hole (#1817 Q12). The fallback
// also covers an image-service URL that fails to load.
const AiringArtwork = ({ airing, channel, sizes }: AiringArtworkProps) => {
  if (airing.thumbImage) {
    return (
      <Image
        image={airing.thumbImage}
        alt=""
        sizes={sizes}
        fallback={<ArtworkFallback channel={channel} />}
        className="size-full object-cover"
      />
    );
  }
  if (airing.thumbUrl) return <img src={airing.thumbUrl} alt="" className="size-full object-cover" />;
  return <ArtworkFallback channel={channel} />;
};

export { AiringArtwork };

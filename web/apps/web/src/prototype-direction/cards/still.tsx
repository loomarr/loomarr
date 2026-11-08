// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
import { standInImage } from "../shared";

// A token-drawn stand-in picture filling its box (the box sets the ratio; the picture crops).
const Still = ({ hue, poster }: { hue: number; poster?: boolean }) => (
  <img
    alt=""
    className="size-full object-cover"
    src={poster ? standInImage(hue, undefined, 400, 600).src : standInImage(hue).src}
  />
);

export { Still };

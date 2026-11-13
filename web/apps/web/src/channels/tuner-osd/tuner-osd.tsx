import type { TunerOSDProps } from "./tuner-osd.type";

// TunerOSD is the synchronous acknowledgement of a tune request (§9.1 V57), for a screen reader. It
// names the target from the already-loaded surf catalog while transport catches up; it does not own
// media state. Nothing of it is drawn: the tuner's wash shows the channel line on screen, and a
// card beside it would say the same thing twice (#1620, #2769).
const TunerOSD = ({ number, name, currentTitle }: TunerOSDProps) => (
  <div role="status" aria-live="polite" className="sr-only">
    <p>CH {number}</p>
    <p>{name}</p>
    {currentTitle && <p>{currentTitle}</p>}
    <p>Tuning…</p>
  </div>
);

export { TunerOSD };

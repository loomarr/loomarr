import type { TaxonomyAxis as Axis } from "../../taxonomy-axes";

const AXIS_COPY: Record<Axis, { label: string; help: string; example: string }> = {
  product: {
    label: "Products & topics",
    help: "What the clip is about or advertising.",
    example: "For example: cereal, cars, local businesses",
  },
  format: {
    label: "Format",
    help: "Descriptive browsing tags. Clip kind separately controls how Loomarr may play it.",
    example: "For example: commercial, promo, station ident",
  },
  presentation: {
    label: "Presentation",
    help: "How the pictures are made, separate from what the clip is about or who may watch it.",
    example: "For example: animated, live action",
  },
  seasonal: {
    label: "Seasonal",
    help: "A holiday or time-of-year cue.",
    example: "For example: Christmas, Halloween, back to school",
  },
  "audience-cue": {
    label: "Audience cues",
    help: "Signals useful for matching a channel's audience.",
    example: "For example: kids-oriented, family, late night",
  },
};

export { AXIS_COPY };

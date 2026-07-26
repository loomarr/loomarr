// The Loomarr component library (frontend-design §4.1 Layer 2), grouped by DOMAIN.
//
// Pages import from here — `import { ChannelCard, GuideGrid } from "@/components/loomarr"` —
// so the folder layout can change without touching a single page. The domains exist because a
// flat directory of 46 components stopped being navigable: the old names compensated with
// prefixes (`channel-*`, `guide-*`, `settings-*`), which is a naming convention doing a
// filesystem's job.
export * from "./ai";
export * from "./channels";
export * from "./feedback";
export * from "./filler";
export * from "./guide";
export * from "./people";
export * from "./settings";
export * from "./setup";
export * from "./shell";

// The Loomarr component library (frontend-design §4.1 Layer 2), grouped by DOMAIN.
//
// This app-wide catalog is for tests and tooling. Production pages import the nearest component
// leaf so route code splitting does not preload every Loomarr component (frontend-design §4.4).
// The domains exist because a flat directory of 46 components stopped being navigable: the old
// names compensated with prefixes (`channel-*`, `guide-*`, `settings-*`), which is a naming
// convention doing a filesystem's job.
export * from "./ai";
export * from "./channels";
export * from "./dashboard";
export * from "./feedback";
export * from "./filler";
export * from "./guide";
export * from "./people";
export * from "./settings";
export * from "./setup";
export * from "./shell";

import type { accents, semanticAliases } from "./tokens";

type Accent = keyof typeof accents;
type Semantic = keyof typeof semanticAliases;

export type { Accent, Semantic };

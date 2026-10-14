import { defineCollection } from "astro:content";
import { docsSchema } from "@astrojs/starlight/schema";
import { repoDocs } from "./loaders/repo-docs.mjs";
import { PUBLISHED } from "./site-pages.mjs";

// ⚠ `base` points OUT of this project, at the repository's real docs/ tree. Nothing is copied
// in — see src/loaders/repo-docs.mjs for why that constraint is load-bearing. What is
// published is src/site-pages.mjs.
export const collections = {
  docs: defineCollection({
    loader: repoDocs({ base: "../docs", include: PUBLISHED }),
    schema: docsSchema(),
  }),
};

import { defineCollection } from "astro:content";
import { docsSchema } from "@astrojs/starlight/schema";
import { repoDocs } from "./loaders/repo-docs.mjs";

// ⚠ `base` points OUT of this project, at the repository's real docs/ tree. Nothing is copied
// in — see src/loaders/repo-docs.mjs for why that constraint is load-bearing.
//
// The include list is explicit rather than a bare glob so that engineering notes and the
// archive stay off the published site. They are internal records: accurate, long, and written
// for the build team. Publishing them would bury the pages a reader actually wants.
export const collections = {
  docs: defineCollection({
    loader: repoDocs({
      base: "../docs",
      include: [
        "install",
        "help",
        "dev",
        "configuration.md",
        "design.md",
        "programming-design.md",
        "config-design.md",
        "frontend-design.md",
        "integrations",
      ],
    }),
    schema: docsSchema(),
  }),
};

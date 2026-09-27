import { defineCollection } from "astro:content";
import { docsSchema } from "@astrojs/starlight/schema";
import { repoDocs } from "./loaders/repo-docs.mjs";

// PROTOTYPE: read the sample tree in place, exactly as the real site reads docs/.
export const collections = {
  docs: defineCollection({
    loader: repoDocs({
      base: "../proto-docs",
      include: ["get-started.md", "guides", "reference", "explanation", "contributing"],
    }),
    schema: docsSchema(),
  }),
};

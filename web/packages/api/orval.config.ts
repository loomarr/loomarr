import { defineConfig } from "orval";

// Generates types + TanStack Query hooks from the committed OpenAPI spec (main
// doc §12/§14; frontend-design §4.2). The spec is the single source of truth —
// contract changes become TypeScript compile errors, no hand-written glue. Output
// is committed and `make fe` regenerates it; CI fails on drift (like openapi.yaml).
export default defineConfig({
  loomarr: {
    input: {
      target: "../../../api/openapi.yaml",
    },
    output: {
      mode: "tags-split", // one file per OpenAPI tag (channels, suggestions, …)
      target: "./generated/endpoints",
      schemas: "./generated/model",
      client: "react-query",
      httpClient: "fetch",
      clean: true,
      prettier: false,
      override: {
        mutator: {
          path: "./src/mutator.ts",
          name: "customFetch",
        },
        query: {
          useQuery: true,
          useInfinite: false,
          useMutation: true,
        },
      },
    },
  },
});

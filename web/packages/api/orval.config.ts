import { defineConfig } from "orval";

// Generates types + TanStack Query hooks from the committed OpenAPI spec (main
// doc §12/§14; frontend-design §4.2). The spec is the single source of truth —
// contract changes become TypeScript compile errors, no hand-written glue. Output
// is committed and `make fe` regenerates it; CI fails on drift (like openapi.yaml).
export default defineConfig({
  // Zod schemas from the SAME spec. These exist so the form schemas in packages/core can DERIVE
  // their wire field names instead of mirroring them by hand (§14). `intentSchema` once said
  // `maxAcquire` where the wire says `maxAcquisitions` and a user's acquisition cap silently
  // vanished; a picked key that is not on the wire is now a compile error at the schema itself.
  //
  // ⚠ Generation carries NAMES and TYPES, not validation. This spec declares almost no
  // constraints (5 `minimum`, 3 `maximum`, 7 `minLength` across ~9k lines, and `maxAcquisitions`
  // has no bounds at all), so every product rule — trim, lengths, the 0–200 cap — and every
  // user-facing message stays hand-authored in packages/core. See schemas.ts.
  loomarrZod: {
    input: {
      target: "../../../api/openapi.yaml",
    },
    output: {
      mode: "tags-split", // mirrors the client's one-file-per-tag layout
      target: "./generated/zod",
      client: "zod",
      fileExtension: ".zod.ts",
      clean: true,
      prettier: false,
      override: {
        zod: {
          // Request-side schemas are what the forms compose from; `response` backs the
          // dev/test-only validation in the mutator.
          generate: { body: true, param: true, query: true, response: true },
        },
      },
    },
  },
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
          path: "./src/mutator/mutator.ts",
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

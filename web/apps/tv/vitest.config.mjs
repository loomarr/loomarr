import base from "../../vitest.universal.config.js";

// tests/*.test.mjs are node:test files (the app's configuration); vitest runs the TypeScript ones.
export default { ...base, test: { include: ["tests/**/*.test.{ts,tsx}"] } };

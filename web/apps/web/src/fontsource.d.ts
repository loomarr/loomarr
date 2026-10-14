// The self-hosted Geist packages (§2.2) are pure CSS — `main` is `index.css`, and they ship
// no `types` field and no `.d.ts`. `main.tsx` imports them for their side effect (Vite
// bundles the font files); there is no value to import and nothing to describe.
//
// ⚠ TypeScript 5 accepted that silently. TypeScript 7 reports TS2882 for a side-effect
// import with no declarations, which is the more honest answer — the declarations were
// always missing, and the old compiler was papering over it.
//
// `vite/client` (in tsconfig `types`) covers `*.css` and the asset globs, but not a bare
// package specifier like `@fontsource-variable/geist`, so these two need stating.
declare module "@fontsource-variable/geist";
declare module "@fontsource-variable/geist-mono";

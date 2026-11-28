import { LoomarrProvider, type LoomarrTheme } from "@loomarr/design-system";
import { type RenderResult, render } from "@testing-library/react";
import type { PropsWithChildren, ReactNode } from "react";

type RenderUiOptions = { theme?: LoomarrTheme };

/**
 * Mounts a node inside LoomarrProvider (dark unless asked). The provider is a wrapper, so
 * `rerender` keeps it. Suites call `cleanup` in `afterEach`: vitest globals are off, so
 * Testing Library does not unmount by itself.
 */
const renderUi = (node: ReactNode, { theme = "dark" }: RenderUiOptions = {}): RenderResult =>
  render(node, {
    wrapper: ({ children }: PropsWithChildren) => <LoomarrProvider theme={theme}>{children}</LoomarrProvider>,
  });

export { cleanup } from "@testing-library/react";
export type { RenderUiOptions };
export { renderUi };

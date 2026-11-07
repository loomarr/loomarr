import { LoomarrProvider } from "@loomarr/design-system";
import { type RenderOptions, render as rtlRender } from "@testing-library/react";
import { Fragment, type PropsWithChildren, type ReactNode } from "react";

// Testing Library's render inside LoomarrProvider, as main.tsx mounts the app: a design-system view
// needs its theme. A test's own wrapper goes inside the provider.
const render = (ui: ReactNode, options: RenderOptions = {}) => {
  const Wrapper = options.wrapper ?? Fragment;
  return rtlRender(ui, {
    ...options,
    wrapper: ({ children }: PropsWithChildren) => (
      <LoomarrProvider theme="dark">
        <Wrapper>{children}</Wrapper>
      </LoomarrProvider>
    ),
  });
};

export { render };

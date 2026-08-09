import type { useRender } from "@base-ui/react/use-render";
import type { VariantProps } from "class-variance-authority";
import type { buttonVariants } from "./button";

// ⚠ The old Radix composition prop is GONE — it is `render` now (retired-ok), and the rename is
// deliberate rather than cosmetic. The old name described its mechanism: `Slot` merged props onto the
// single CHILD. Base UI takes the element as a PROP instead, so keeping a name that promises a
// child would be exactly the half-migrated vocabulary that outlives whoever introduced it. Every
// composed trigger in the app now reads the same way:
//
//   <Button render={<Link to="/filler" />}>Add clips</Button>
//
// (Base UI's own components carry a `nativeButton` escape hatch for this case. This Button is
// built directly on `useRender`, not on Base UI's Button, so there is nothing assuming button
// semantics to switch off — `render` alone is the whole contract.)
interface ButtonProps extends useRender.ComponentProps<"button">, VariantProps<typeof buttonVariants> {}

export type { ButtonProps };

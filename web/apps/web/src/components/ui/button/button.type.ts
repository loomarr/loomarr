import type { VariantProps } from "class-variance-authority";
import type * as React from "react";
import type { buttonVariants } from "./button";

interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export type { ButtonProps };

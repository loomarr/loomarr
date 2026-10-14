import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

// The shadcn class-merge helper: clsx for conditional composition, tailwind-merge
// to resolve conflicting utilities so later wins (e.g. `px-2` + `px-4` → `px-4`).
const cn = (...inputs: ClassValue[]): string => twMerge(clsx(inputs));

export { cn };

import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn merges Tailwind class names, resolving conflicts (shadcn-style).
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

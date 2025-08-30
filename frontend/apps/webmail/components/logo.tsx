import { cn } from "@/lib/utils";

// Logo renders the mailez brand icon as a rounded tile.
export function Logo({ className }: { className?: string }) {
  return (
    // Static local SVG: next/image adds nothing for a tiny inline brand icon.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src="/mailez-icon.svg"
      alt="Mailez"
      className={cn("size-8 shrink-0 rounded-lg", className)}
    />
  );
}

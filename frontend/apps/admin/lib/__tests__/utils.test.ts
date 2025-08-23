import { describe, expect, it } from "vitest";

import { cn } from "@/lib/utils";

describe("cn", () => {
  it("merges class names and resolves tailwind conflicts", () => {
    expect(cn("a", "b", false && "c", undefined)).toBe("a b");
    expect(cn("px-2", "px-4")).toBe("px-4");
  });
});

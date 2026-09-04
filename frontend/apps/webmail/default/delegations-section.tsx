"use client";

import { ModulePlaceholder } from "./placeholder";

// Default module set: no delegation management module ships here. The call
// site passes the full prop shape; the stub ignores it.
export function DelegationsSection(_props: Record<string, unknown>) {
  return <ModulePlaceholder label="代发 / 代管" />;
}

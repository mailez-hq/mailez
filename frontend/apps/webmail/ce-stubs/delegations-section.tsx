"use client";

import { EnterprisePlaceholder } from "./placeholder";

// Community stub: delegation management ships with the enterprise edition.
// The call site passes the full enterprise prop shape; the stub ignores it.
export function DelegationsSection(_props: Record<string, unknown>) {
  return <EnterprisePlaceholder label="代发 / 代管" />;
}

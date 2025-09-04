"use client";

import { ModulePlaceholder } from "./placeholder";

// Default module set: no S/MIME management module ships here.
export function SmimeSection(_props: Record<string, unknown>) {
  return <ModulePlaceholder label="S/MIME" />;
}

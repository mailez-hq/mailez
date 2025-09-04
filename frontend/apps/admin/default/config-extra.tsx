"use client";

// Default module set: the branding, AI provider and directory integration
// tabs are provided by an optional config module. This set exports no extra
// tabs, so they disappear from the shared config page and nothing renders.
export const EXTRA_TABS: string[] = [];

export function ExtraConfigPanel(_props: { tab: string }) {
  return null;
}

"use client";

// Default module set: the overview renders the shared infrastructure and
// usage cards only; no extra cards ship here.
import type { AdminOverview } from "@/lib/api";

export function OverviewExtras(_props: { data: AdminOverview }) {
  return null;
}

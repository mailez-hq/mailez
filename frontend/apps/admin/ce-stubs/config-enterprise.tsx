"use client";

// Community stub: branding, AI providers and AD/LDAP integration are
// enterprise-only config surfaces, so CE exposes no enterprise tabs and the
// shared config page renders nothing for them.
export const ENTERPRISE_TABS: string[] = [];

export function EnterpriseConfigPanel(_props: { tab: string }) {
  return null;
}

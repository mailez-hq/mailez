// Shared mailez API contract types. Both frontend apps import from here so
// the wire shapes stay in sync; the long-term source of truth is the backend
// OpenAPI document (generated types replace these by hand).
export * from "./contract/auth";
export * from "./contract/admin";

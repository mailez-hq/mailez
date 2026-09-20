// Contacts, calendar and invitation API calls.
import type {
  Contact,
  CalendarEvent,
  CalendarEventInput,
  CalendarShare,
  CalendarShareListing,
  OrgContact,
} from "@mailez/types";

import {
  api,
  apiPost,
  apiPut,
  ApiError,
  HAS_OPTIONAL,
  handleUnauthorized,
  sessionExpiredText,
  API,
} from "./api-client";

// Contacts.
export const contactsDedupe = () => apiPost<{merged: number; removed: number}>("/contacts/dedupe", {});

export const carddavGet = () => api<{url: string; username: string; has_auth: boolean}>("/contacts/carddav");

export const carddavSet = (body: {url: string; username?: string; password?: string}) =>
  apiPut<void>("/contacts/carddav", body);

export const carddavSync = () => apiPost<{added: number; updated: number; total: number}>("/contacts/carddav/sync", {});

// Calendar (webmail JSON API over the same store as CalDAV).
export const calendarEvents = (from?: string, to?: string) => {
  const q = new URLSearchParams();
  if (from) q.set("from", from);
  if (to) q.set("to", to);
  const qs = q.toString();
  return api<CalendarEvent[]>(`/calendar/events${qs ? `?${qs}` : ""}`);
};

export const calendarEventCreate = (input: CalendarEventInput) =>
  apiPost<CalendarEvent>("/calendar/events", input);

export const calendarEventUpdate = (id: number, input: CalendarEventInput) =>
  apiPut<CalendarEvent>(`/calendar/events/${id}`, input);

export const calendarEventDelete = (id: number) =>
  api<void>(`/calendar/events/${id}`, { method: "DELETE" });

// Calendar sharing and ICS subscription.
// Calendar sharing ships with the extended module set; without it the
// listing resolves locally (mirrors the default backend set).
export const calendarShares = (): Promise<CalendarShareListing> =>
  HAS_OPTIONAL
    ? api<CalendarShareListing>("/calendar/shares")
    : Promise.resolve({ owned: [], granted: [] });

export const calendarShareCreate = (shareeEmail: string, readOnly: boolean) =>
  apiPost<CalendarShare>("/calendar/shares", { sharee_email: shareeEmail, read_only: readOnly });

export const calendarShareDelete = (id: number) =>
  api<void>(`/calendar/shares/${id}`, { method: "DELETE" });

export const calendarFeed = () => api<{ url: string }>("/calendar/feed");

// Meeting invitations (iTIP): respond to a received REQUEST or send a new
// one to attendees.
export const inviteRespond = (ics: string, action: "accept" | "decline" | "tentative" | "cancel") =>
  apiPost<{ ok: boolean; removed?: boolean }>("/invites/respond", { ics, action });

export const inviteSend = (input: {
  to: string[];
  summary: string;
  location: string;
  description: string;
  start: string;
  end?: string;
}) => apiPost<{ ok: boolean }>("/invites/send", input);

// Organization address book (read-only, synced from AD/LDAP).
export const orgContacts = () => api<OrgContact[]>("/contacts/org");

// address book
export const contacts = () => api<Contact[]>("/contacts");

export const createContact = (name: string, email: string, comment = "", groups = "", avatar = "") =>
  apiPost<Contact>("/contacts", { name, email, comment, groups, avatar });

export const updateContact = (
  id: number,
  body: Partial<Pick<Contact, "name" | "email" | "comment" | "groups" | "avatar">>,
) => apiPut<Contact>(`/contacts/${id}`, body);

export const deleteContact = (id: number) =>
  api(`/contacts/${id}`, { method: "DELETE" });

export const exportContacts = async (): Promise<string> => {
  const res = await fetch(`${API}/contacts/export`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const expired = res.status === 401;
    if (expired) handleUnauthorized("/contacts/export");
    throw new ApiError(
      expired ? sessionExpiredText() : (body as { error?: string }).error || res.statusText,
      res.status,
    );
  }
  return res.text();
};

export const importContacts = (data: string) =>
  apiPost<{ added: number; total: number }>("/contacts/import", { data });

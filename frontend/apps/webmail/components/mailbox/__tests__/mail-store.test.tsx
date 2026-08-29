import { act, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MailMessage } from "@mailez/types";

// The store destructures the full api surface at import time, so the mock
// factory must provide every name. Flows under test override the fakes they
// care about; the rest only need to exist and resolve benignly.
const api = vi.hoisted(() => {
  const ok = async () => true;
  return {
    accounts: vi.fn(async () => []),
    delegations: vi.fn(async () => ({ received: [] })),
    setActiveAccountId: vi.fn(),
    setActiveDelegateEmail: vi.fn(),
    contacts: vi.fn(async () => []),
    mailFlag: vi.fn(ok),
    mailMove: vi.fn(ok),
    mailIdentities: vi.fn(async () => []),
    logout: vi.fn(ok),
    mailFolders: vi.fn(async () => ["Inbox", "Sent", "Trash"]),
    mailFolderCreate: vi.fn(ok),
    mailFolderRename: vi.fn(ok),
    mailFolderDelete: vi.fn(ok),
    mailFolderClear: vi.fn(ok),
    mailLabelDelete: vi.fn(ok),
    mailLabelRename: vi.fn(async () => ({ name: "", color: "" })),
    mailLabelSave: vi.fn(async (name: string, color: string) => ({ name, color })),
    mailLabels: vi.fn(async () => []),
    mailMessage: vi.fn(async () => null),
    mailMessages: vi.fn(async (): Promise<{ messages: MailMessage[]; total: number }> => ({ messages: [], total: 0 })),
    mailSaveDraft: vi.fn(async () => ({ id: "d1" })),
    mailSearch: vi.fn(async () => []),
    mailSearchSpec: vi.fn(async (): Promise<MailMessage[]> => []),
    mailSend: vi.fn(ok),
    mailSendReply: vi.fn(ok),
    mailThread: vi.fn(async () => ({ thread_id: "t1", messages: [] })),
    mailUnseen: vi.fn(async () => ({ Inbox: 3 })),
    mailUndoSend: vi.fn(ok),
    mailUnsubscribe: vi.fn(ok),
    mailScheduled: vi.fn(async () => []),
    mailDelete: vi.fn(ok),
    mailSnooze: vi.fn(ok),
    mailSnoozed: vi.fn(async () => []),
    mailReceipt: vi.fn(ok),
    mailRecall: vi.fn(ok),
    mailRecallApply: vi.fn(ok),
    mailMerge: vi.fn(ok),
    mailReadAll: vi.fn(ok),
    aiReplies: vi.fn(async () => []),
    uploadLargeAttachment: vi.fn(async () => ({})),
    meProfile: vi.fn(async () => ({ whitelist: "" })),
    updateMeSettings: vi.fn(ok),
    pgpEncrypt: vi.fn(async () => ""),
    pgpLookup: vi.fn(async () => ""),
    pgpSign: vi.fn(async () => ""),
    aiStatus: vi.fn(async () => ({ enabled: false })),
    aiSummarize: vi.fn(async () => ""),
    aiDraft: vi.fn(async () => ""),
    aiDraftNew: vi.fn(async () => ""),
    aiPrioritize: vi.fn(async () => ({})),
    aiSearch: vi.fn(async () => null),
    aiComposeDraft: vi.fn(async () => ""),
    aiComposeStream: vi.fn(async () => ""),
  };
});

vi.mock("@/lib/api", () => api);
vi.mock("@/lib/events", () => ({ subscribeMailEvents: vi.fn(() => () => {}) }));
vi.mock("@/lib/push", () => ({
  setupPushSubscription: vi.fn(async () => undefined),
  teardownPushSubscription: vi.fn(async () => undefined),
}));
vi.mock("@/components/palette/palette-actions", () => ({ usePaletteActions: () => [] }));

import { renderMailStore } from "@/test/test-utils";

function makeMsg(over: Partial<MailMessage> = {}): MailMessage {
  return {
    uid: 1,
    id: "m1",
    seq: 1,
    subject: "Hello",
    from: [{ name: "Bob", email: "bob@x.com" }],
    to: [{ name: "Alice", email: "alice@example.com" }],
    date: "2026-01-01T00:00:00Z",
    flags: [],
    has_attachment: false,
    ...over,
  };
}

async function navPush() {
  const nav = (await import("next/navigation")) as unknown as { __mockPush: ReturnType<typeof vi.fn> };
  return nav.__mockPush;
}

const INBOX_MESSAGES = [
  makeMsg({ uid: 1, id: "m1", seq: 1, subject: "First" }),
  makeMsg({ uid: 2, id: "m2", seq: 2, subject: "Second", flags: ["\\Seen"] }),
  makeMsg({ uid: 3, id: "m3", seq: 3, subject: "Third" }),
];

beforeEach(() => {
  vi.clearAllMocks();
  api.mailMessages.mockResolvedValue({ messages: INBOX_MESSAGES, total: 3 });
});

describe("MailStore (characterization)", () => {
  it("loads folders, unseen counts and the initial message list on mount", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));
    expect(observe().messages.map((m: MailMessage) => m.subject)).toEqual(["First", "Second", "Third"]);
    expect(observe().folders).toEqual(["Inbox", "Sent", "Trash"]);
    await waitFor(() => expect(api.mailUnseen).toHaveBeenCalled());
    expect(api.mailFolders).toHaveBeenCalled();
    expect(api.mailMessages).toHaveBeenCalledWith("Inbox", 0, "date", "desc", expect.any(Boolean));
  });

  it("openMessage marks the message seen and navigates to its route", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      observe().openMessage(INBOX_MESSAGES[0]);
    });

    expect(api.mailFlag).toHaveBeenCalledWith("Inbox", 1, "\\Seen", true);
    const push = await navPush();
    expect(push).toHaveBeenCalledWith("/mail/Inbox/m1");
    // The list row flips to seen optimistically.
    await waitFor(() => {
      const row = observe().messages.find((m: MailMessage) => m.uid === 1);
      expect(row?.flags).toContain("\\Seen");
    });
  });

  it("openMessage on an already-seen message does not refire the seen flag", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      observe().openMessage(INBOX_MESSAGES[1]);
    });
    expect(api.mailFlag).not.toHaveBeenCalled();
  });

  it("toggleStar flips the \\Flagged flag through the API and in the list", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      await observe().toggleStar(INBOX_MESSAGES[0]);
    });

    expect(api.mailFlag).toHaveBeenCalledWith("Inbox", 1, "\\Flagged", true);
    await waitFor(() => {
      const row = observe().messages.find((m: MailMessage) => m.uid === 1);
      expect(row?.flags).toContain("\\Flagged");
    });

    await act(async () => {
      await observe().toggleStar({ ...INBOX_MESSAGES[0], flags: ["\\Flagged"] });
    });
    expect(api.mailFlag).toHaveBeenLastCalledWith("Inbox", 1, "\\Flagged", false);
  });

  it("bulkDelete moves selected messages to Trash and offers undo that moves them back", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      observe().toggleSelect(INBOX_MESSAGES[0]);
      observe().toggleSelect(INBOX_MESSAGES[2]);
    });
    expect([...observe().selectedUids]).toEqual([1, 3]);

    await act(async () => {
      await observe().bulkDelete();
    });

    expect(api.mailMove).toHaveBeenCalledWith("Inbox", [1, 3], "Trash");
    // Moved rows leave the list, selection clears, toast offers undo.
    expect(observe().messages.map((m: MailMessage) => m.uid)).toEqual([2]);
    expect([...observe().selectedUids]).toEqual([]);
    expect(observe().toast).not.toBeNull();

    const undo = observe().toast?.onUndo;
    expect(typeof undo).toBe("function");
    await act(async () => {
      undo?.();
    });
    expect(api.mailMove).toHaveBeenCalledWith("Trash", [1, 3], "Inbox");
  });

  it("bulkFlag flags every selected message and clears the selection", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      observe().toggleSelect(INBOX_MESSAGES[0]);
      observe().toggleSelect(INBOX_MESSAGES[1]);
    });
    await act(async () => {
      await observe().bulkFlag("\\Flagged", true);
    });

    expect(api.mailFlag).toHaveBeenCalledWith("Inbox", 1, "\\Flagged", true);
    expect(api.mailFlag).toHaveBeenCalledWith("Inbox", 2, "\\Flagged", true);
    expect([...observe().selectedUids]).toEqual([]);
    await waitFor(() => {
      expect(observe().messages.find((m: MailMessage) => m.uid === 1)?.flags).toContain("\\Flagged");
    });
  });

  it("doSearch routes a keyword query through the structured search endpoint", async () => {
    api.mailSearchSpec.mockResolvedValue([makeMsg({ uid: 9, id: "m9", subject: "Found" })]);
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      await observe().doSearch("from:bob invoice");
    });

    expect(api.mailSearchSpec).toHaveBeenCalledWith(
      "Inbox",
      expect.objectContaining({ from: ["bob"], text: ["invoice"] }),
    );
    expect(observe().searching).toBe(true);
    expect(observe().messages.map((m: MailMessage) => m.subject)).toEqual(["Found"]);
  });

  it("clearSearch drops search state and reloads the folder list", async () => {
    api.mailSearchSpec.mockResolvedValue([makeMsg({ uid: 9, id: "m9", subject: "Found" })]);
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      await observe().doSearch("invoice");
    });
    expect(observe().searching).toBe(true);

    await act(async () => {
      await observe().clearSearch();
    });

    expect(observe().searching).toBe(false);
    expect(observe().query).toBe("");
    expect(api.mailMessages).toHaveBeenCalledWith("Inbox", 0, "date", "desc", expect.any(Boolean));
    await waitFor(() => {
      expect(observe().messages.map((m: MailMessage) => m.subject)).toEqual(["First", "Second", "Third"]);
    });
  });

  it("changeSort updates sort state and reloads the list silently", async () => {
    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));

    await act(async () => {
      observe().changeSort("subject-asc");
    });

    expect(observe().sortBy).toBe("subject");
    expect(observe().sortDir).toBe("asc");
    expect(api.mailMessages).toHaveBeenCalledWith("Inbox", 0, "subject", "asc", expect.any(Boolean));
  });

  it("openCompose seeds recipients and closeCompose dismisses the panel", async () => {    const { observe } = renderMailStore(null);
    await waitFor(() => expect(observe().loading).toBe(false));
    expect(observe().composeOpen).toBe(false);

    await act(async () => {
      observe().openCompose("bob@x.com, carol@x.com", "Quarterly report");
    });

    expect(observe().composeOpen).toBe(true);
    expect(observe().to).toEqual(["bob@x.com", "carol@x.com"]);
    expect(observe().subject).toBe("Quarterly report");
    expect(observe().composeError).toBe("");

    await act(async () => {
      observe().closeCompose();
    });
    expect(observe().composeOpen).toBe(false);
  });
});

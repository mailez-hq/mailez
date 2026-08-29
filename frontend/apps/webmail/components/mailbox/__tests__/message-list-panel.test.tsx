import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MailMessage } from "@mailez/types";

import { MessageListPanel } from "@/components/mailbox/message-list-panel";
import { renderMailStore } from "@/test/test-utils";

afterEach(cleanup);

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

function baseProps(over: Record<string, unknown> = {}) {
  return {
    folder: "Inbox",
    messages: [],
    displayMessages: [],
    total: 0,
    searching: false,
    loading: false,
    query: "",
    searchSpec: null,
    onQueryChange: vi.fn(),
    onApplySpec: vi.fn(),
    onSearch: vi.fn(),
    onClearSearch: vi.fn(),
    selectedUids: new Set<number>(),
    cursor: 0,
    onOpen: vi.fn(),
    onToggleSelect: vi.fn(),
    onDelete: vi.fn(),
    onStar: vi.fn(),
    onArchive: vi.fn(),
    onBulkDelete: vi.fn(),
    onBulkArchive: vi.fn(),
    onBulkSpam: vi.fn(),
    onBulkFlag: vi.fn(),
    onLoadMore: vi.fn(),
    searchInputRef: { current: null },
    onMenu: vi.fn(),
    error: "",
    folders: ["Inbox"],
    onMoveToFolder: vi.fn(),
    onSaveSearch: vi.fn(),
    onSaveSearchSpec: vi.fn(),
    searchAll: false,
    onToggleSearchAll: vi.fn(),
    category: "",
    onCategoryChange: vi.fn(),
    aiSearchEnabled: false,
    aiPriorityEnabled: false,
    prioritizing: false,
    priorityOn: false,
    onTogglePriority: vi.fn(),
    aiSearching: false,
    onAiSearch: vi.fn(),
    refreshing: false,
    onRefresh: vi.fn(),
    sortBy: "date",
    sortDir: "desc",
    onChangeSort: vi.fn(),
    onBulkLabel: vi.fn(),
    onMarkAllRead: vi.fn(),
    ...over,
  } as Parameters<typeof MessageListPanel>[0];
}

describe("MessageListPanel", () => {
  it("renders one row per message with sender and subject", () => {
    renderMailStore(<MessageListPanel
      {...baseProps({
        messages: [
          makeMsg({ uid: 1, subject: "First" }),
          makeMsg({ uid: 2, id: "m2", seq: 2, subject: "Second" }),
        ],
        displayMessages: [
          makeMsg({ uid: 1, subject: "First" }),
          makeMsg({ uid: 2, id: "m2", seq: 2, subject: "Second" }),
        ],
        total: 2,
      })}
    />);
    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.getByText("Second")).toBeInTheDocument();
  });

  it("shows the empty state when the folder has no messages", () => {
    renderMailStore(<MessageListPanel {...baseProps()} />);
    expect(screen.getByText("noMessages")).toBeInTheDocument();
  });

  it("opens a message when its row is clicked", () => {
    const onOpen = vi.fn();
    const msg = makeMsg({ uid: 7, id: "m7", subject: "Clickable" });
    renderMailStore(<MessageListPanel {...baseProps({ messages: [msg], displayMessages: [msg], total: 1, onOpen })} />);

    fireEvent.click(screen.getByText("Clickable"));
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ uid: 7 }));
  });

  it("flags a message from its row star control", () => {
    const onStar = vi.fn();
    const msg = makeMsg({ uid: 7, id: "m7", subject: "Star me" });
    renderMailStore(<MessageListPanel {...baseProps({ messages: [msg], displayMessages: [msg], total: 1, onStar })} />);

    fireEvent.click(screen.getByTitle("star"));
    expect(onStar).toHaveBeenCalledWith(expect.objectContaining({ uid: 7 }));
  });
});

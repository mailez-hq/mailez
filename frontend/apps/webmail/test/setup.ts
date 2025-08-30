// Type-level augmentation only: this import makes tsc know the DOM
// matchers on vitest's Assertion. Its runtime side effect (extending the
// expect instance it resolves) did not reach the test files under
// vitest 4, hence the explicit extend below — keep both.
import "@testing-library/jest-dom/vitest";

// Explicit extend instead of "@testing-library/jest-dom/vitest": with
// vitest 4 the side-effect import resolves a different expect instance in
// some module graphs and the DOM matchers never reach the test files.
import * as jestDomMatchers from "@testing-library/jest-dom/matchers";

import { vi, expect as vitestExpect } from "vitest";

vitestExpect.extend(jestDomMatchers);

// Components under test call useTranslations("mail") etc. Behavior tests
// assert on state and semantics, not copy, so echoing the key keeps tests
// independent of the message catalogs. Add richer fakes here if a test ever
// needs to match on interpolated text.
vi.mock("next-intl", () => ({
  useTranslations: () => {
    // Echo the key so behavior tests never depend on catalog copy.
    const fn = (key: string, values?: Record<string, unknown>) => {
      if (!values) return key;
      const parts = Object.values(values).map((v) => String(v));
      return parts.length > 0 ? `${key} ${parts.join(" ")}` : key;
    };
    // Helpers the real next-intl translator carries (folder-tree uses .has).
    (fn as unknown as { has: (key: string) => boolean }).has = () => false;
    (fn as unknown as { rich: (key: string) => string }).rich = (key: string) => key;
    (fn as unknown as { markup: (key: string) => string }).markup = (key: string) => key;
    return fn;
  },
}));

// MailStoreProvider and friends read the router solely to navigate after
// account switches / folder changes. The app-router shape below matches what
// the components consume; individual tests can reach the fakes via
// requireMock if they need to assert on navigation calls.
vi.mock("next/navigation", () => {
  const push = vi.fn();
  const replace = vi.fn();
  const back = vi.fn();
  const prefetch = vi.fn();
  return {
    useRouter: () => ({ push, replace, back, prefetch, refresh: vi.fn() }),
    usePathname: () => "/mail/Inbox",
    useSearchParams: () => new URLSearchParams(),
    // exposed for assertions in tests that care
    __mockPush: push,
    __mockReplace: replace,
  };
});

// jsdom (v30) exposes a non-callable matchMedia placeholder, so override it
// unconditionally with a working fake.
if (typeof window !== "undefined") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  });
}

// ---- global @/lib/api mock ----
// Component tests mount the real MailStoreProvider, which pulls the whole api
// surface at import time. The global mock provides every export with benign
// implementations; a test file can re-declare vi.mock("@/lib/api", ...) to
// override it for specific fixtures (see mail-store.test.tsx).
vi.mock("@/lib/api", () => {
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
    mailMessages: vi.fn(async () => ({ messages: [], total: 0 })),
    mailSaveDraft: vi.fn(async () => ({ id: "d1" })),
    mailSearch: vi.fn(async () => []),
    mailSearchSpec: vi.fn(async () => []),
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

vi.mock("@/lib/events", () => ({ subscribeMailEvents: vi.fn(() => () => {}) }));
vi.mock("@/lib/push", () => ({
  setupPushSubscription: vi.fn(async () => undefined),
  teardownPushSubscription: vi.fn(async () => undefined),
}));
vi.mock("@/components/palette/palette-actions", () => ({ usePaletteActions: () => [] }));

// jsdom lacks ResizeObserver (used by the virtual message list).
if (typeof window !== "undefined" && typeof window.ResizeObserver === "undefined") {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(window, "ResizeObserver", {
    writable: true,
    configurable: true,
    value: ResizeObserverStub,
  });
}

// jsdom does not implement programmatic scrolling.
if (typeof Element !== "undefined" && !Element.prototype.scrollTo) {
  Element.prototype.scrollTo = () => {};
}
if (typeof Element !== "undefined" && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

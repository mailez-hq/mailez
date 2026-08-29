import { render, renderHook, type RenderHookResult, type RenderResult } from "@testing-library/react";
import { useEffect, type ReactElement, type ReactNode } from "react";

import { PreferencesProvider } from "@/components/preferences-provider";
import { MailStoreProvider, useMailStore } from "@/components/mailbox/mail-store";
import type { Me } from "@/lib/api";

type StoreSnapshot = ReturnType<typeof useMailStore>;

function TestProviders({ children }: { children: ReactNode }) {
  return <PreferencesProvider>{children}</PreferencesProvider>;
}

/** Render a component inside the providers it needs in jsdom. */
export function renderWithProviders(ui: ReactElement): RenderResult {
  return render(ui, { wrapper: TestProviders });
}

/** Render a hook inside the same provider set. */
export function renderHookWithProviders<TResult, TProps>(
  callback: (props: TProps) => TResult,
): RenderHookResult<TResult, TProps> {
  return renderHook(callback, { wrapper: TestProviders });
}

export const FAKE_ME: Me = {
  email: "alice@example.com",
  displayed_name: "Alice",
  global_admin: false,
  manager: false,
  enabled: true,
};

/**
 * Render MailStoreProvider around children that can consume the store via
 * useMailStore(). The lib/api module must be mocked by the calling test
 * (vi.mock("@/lib/api") or the global mock in test/setup.ts).
 */
export function renderMailStore(
  children: ReactNode,
): RenderResult & { observe: () => StoreSnapshot } {
  // The probe mirrors the store value into a holder from an effect (never
  // during render), so observe() can hand tests the latest committed value.
  const holder: { current: StoreSnapshot | null } = { current: null };

  function Probe() {
    const value = useMailStore();
    useEffect(() => {
      holder.current = value;
    });
    return null;
  }

  const view = render(
    <PreferencesProvider>
      <MailStoreProvider me={FAKE_ME}>
        <Probe />
        {children}
      </MailStoreProvider>
    </PreferencesProvider>,
  );

  return {
    ...view,
    observe: () => {
      if (!holder.current) throw new Error("store probe has not rendered yet");
      return holder.current;
    },
  };
}



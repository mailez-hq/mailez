import { NextResponse, type NextRequest } from "next/server";

// A signed-in visitor who lands on the bare sign-in route (address bar,
// bookmark, favourite) must not be shown the login form again. The session
// cookie is HttpOnly, so the client page cannot see it — decide here, on
// the server, from the request cookies.
//
// Only the bare "/" is redirected: deep links carrying ?next= keep their
// client-side probe (the page needs the mailbox-target logic), and ?expired
// must keep rendering the sign-in form with its session-expired notice —
// redirecting that would loop (/home 401s back to /?expired=1).
//
// The redirect target is /home rather than the client's last-folder logic
// (localStorage is invisible here). Should the cookie be stale, /home's
// auth probe bounces back to the sign-in page with ?expired=1 — the same
// flow every other stale entry uses.
export function middleware(req: NextRequest) {
  if (req.nextUrl.searchParams.has("next") || req.nextUrl.searchParams.has("expired")) {
    return NextResponse.next();
  }
  if (req.cookies.get("mailez_session")?.value) {
    return NextResponse.redirect(new URL("/home", req.url));
  }
  return NextResponse.next();
}

export const config = {
  // Match only the exact root route; everything else (including /?next=...)
  // is handled above or not at all.
  matcher: ["/"],
};

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminSignIn from "@/pages/AdminSignIn";
import SignIn from "@/pages/SignIn";
import SignUp from "@/pages/SignUp";

const profile = vi.hoisted(() => ({ needsSetup: false }));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ profile, generalSetting: {}, initialize: vi.fn() }),
}));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ initialize: vi.fn() }) }));

vi.mock("@/components/AuthFooter", () => ({ default: () => null }));
vi.mock("@/components/PasswordSignInForm", () => ({
  default: ({ redirectPath }: { redirectPath?: string }) => <div data-testid="password-sign-in">{redirectPath}</div>,
}));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const renderPage = (path = "/auth") =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <SignIn />
    </MemoryRouter>,
  );

describe("<SignIn>", () => {
  it("shows password sign-in and preserves a safe destination", () => {
    renderPage("/auth?redirect=%2Fmemos%2Fexample");

    expect(screen.getByTestId("password-sign-in")).toHaveTextContent("/memos/example");
    expect(screen.queryByRole("link", { name: "common.sign-up" })).not.toBeInTheDocument();
  });

  it("announces closed registration while allowing existing accounts to sign in", () => {
    renderPage("/auth?reason=signups-closed");

    expect(screen.getByRole("status")).toHaveTextContent("auth.signups-closed-description");
    expect(screen.getByTestId("password-sign-in")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.sign-up" })).not.toBeInTheDocument();
  });

  it("announces an idle session expiry and preserves the destination for re-login", () => {
    renderPage("/auth?reason=idle-session&redirect=%2Fmemos%2Fexample");

    expect(screen.getByRole("status")).toHaveTextContent("auth.session-idle-expired");
    expect(screen.getByTestId("password-sign-in")).toHaveTextContent("/memos/example");
  });

  it("keeps legacy administrator sign-in and its return destination", () => {
    render(
      <MemoryRouter initialEntries={["/auth/admin?redirect=%2Fmemos%2Fexample"]}>
        <AdminSignIn />
      </MemoryRouter>,
    );

    expect(screen.getByTestId("password-sign-in")).toHaveTextContent("/memos/example");
    expect(screen.getByRole("link", { name: "auth.back-to-sign-in" })).toHaveAttribute("href", "/auth?redirect=%2Fmemos%2Fexample");
  });
});

function SignInLocation() {
  const { search, hash } = useLocation();
  return <output data-testid="sign-in-location">{search + hash}</output>;
}

describe("first administrator setup", () => {
  beforeEach(() => {
    profile.needsSetup = false;
  });

  it("redirects the old signup link to sign-in after the instance has been set up", async () => {
    render(
      <MemoryRouter initialEntries={["/auth/signup?redirect=%2Fmemos%2Fexample#anchor"]}>
        <Routes>
          <Route path="/auth/signup" element={<SignUp />} />
          <Route path="/auth" element={<SignInLocation />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByTestId("sign-in-location")).toHaveTextContent("?redirect=%2Fmemos%2Fexample&reason=signups-closed#anchor");
  });

  it("still opens the setup form for a new instance", () => {
    profile.needsSetup = true;
    render(
      <MemoryRouter>
        <SignUp />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "auth.setup-title" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "auth.create-admin-account" })).toBeInTheDocument();
  });
});

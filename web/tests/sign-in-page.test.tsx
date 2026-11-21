import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SignIn from "@/pages/SignIn";

const generalSetting = vi.hoisted(() => ({ disallowPasswordAuth: false, disallowUserRegistration: false }));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ generalSetting }),
}));

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
  beforeEach(() => {
    generalSetting.disallowPasswordAuth = false;
    generalSetting.disallowUserRegistration = false;
  });

  it("shows password sign-in and preserves a safe destination", () => {
    renderPage("/auth?redirect=%2Fmemos%2Fexample");

    expect(screen.getByTestId("password-sign-in")).toHaveTextContent("/memos/example");
    expect(screen.getByRole("link", { name: "common.sign-up" })).toHaveAttribute("href", "/auth/signup?redirect=%2Fmemos%2Fexample");
  });

  it("announces closed registration while allowing existing accounts to sign in", () => {
    generalSetting.disallowUserRegistration = true;
    renderPage("/auth?reason=signups-closed");

    expect(screen.getByRole("status")).toHaveTextContent("auth.signups-closed-description");
    expect(screen.getByTestId("password-sign-in")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.sign-up" })).not.toBeInTheDocument();
  });

  it("offers the administrator sign-in route when password sign-in is disabled for members", () => {
    generalSetting.disallowPasswordAuth = true;
    renderPage();

    expect(screen.queryByTestId("password-sign-in")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "auth.admin-sign-in" })).toHaveAttribute("href", "/auth/admin");
  });
});

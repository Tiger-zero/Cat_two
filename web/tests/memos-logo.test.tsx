import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import MemosLogo from "@/components/MemosLogo";

const mockInstance = {
  generalSetting: {} as { customProfile?: { title?: string; logoUrl?: string } },
};

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => mockInstance,
}));

describe("<MemosLogo>", () => {
  beforeEach(() => {
    mockInstance.generalSetting = {};
  });

  it.each([undefined, "", "/logo.webp", "/logo.svg", "/logo-small.svg"])("uses the cat for the built-in logo URL %s", (logoUrl) => {
    mockInstance.generalSetting = { customProfile: { logoUrl } };
    const { container } = render(<MemosLogo />);

    const logo = container.querySelector("img");
    expect(logo).toHaveAttribute("src", "/logo.webp?v=cat-two");
    expect(logo).toHaveAttribute("alt", "");
    expect(screen.getByText("Memos")).toBeInTheDocument();
  });

  it.each(["/custom-logo.png", "/custom-logo.svg", "https://example.com/logo.webp"])("preserves custom branding at %s", (logoUrl) => {
    mockInstance.generalSetting = { customProfile: { title: "Team Notes", logoUrl } };
    const { container } = render(<MemosLogo />);

    expect(container.querySelector("img")).toHaveAttribute("src", logoUrl);
    expect(container.querySelector(".memos-mark")).toBeNull();
    expect(screen.getByText("Team Notes")).toBeInTheDocument();
  });

  it("names the standalone mark when the title is collapsed", () => {
    render(<MemosLogo collapsed />);

    expect(screen.getByRole("img", { name: "Memos" })).toHaveAttribute("src", "/logo.webp?v=cat-two");
  });

  it("names a custom standalone logo when the title is collapsed", () => {
    mockInstance.generalSetting = { customProfile: { title: "Team Notes", logoUrl: "/custom-logo.svg" } };
    render(<MemosLogo collapsed />);

    expect(screen.getByRole("img", { name: "Team Notes" })).toHaveAttribute("src", "/custom-logo.svg");
  });
});

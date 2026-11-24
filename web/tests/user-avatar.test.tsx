import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import UserAvatar from "@/components/UserAvatar";

describe("UserAvatar", () => {
  it("shows the bundled Cat_two app icon for users without a custom avatar", () => {
    const { container, rerender } = render(<UserAvatar name="Steven" />);
    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
    expect(container.querySelector("img")).toHaveAttribute("src", "/android-chrome-192x192.png?v=cat-two");
    expect(container.querySelector("img")).toHaveAttribute("alt", "");

    rerender(<UserAvatar />);
    expect(container.querySelector("img")).toHaveAttribute("src", "/android-chrome-192x192.png?v=cat-two");
  });

  it("tries the custom avatar first and uses the bundled icon if the custom avatar fails", () => {
    const { container, rerender } = render(<UserAvatar avatarUrl="/avatar.png" name="Steven" />);
    const image = container.querySelector("img")!;
    expect(image).toHaveAttribute("src", "/avatar.png");
    expect(image).toHaveAttribute("loading", "lazy");

    fireEvent.error(image);
    expect(image).toHaveAttribute("src", "/android-chrome-192x192.png?v=cat-two");

    rerender(<UserAvatar avatarUrl="/updated-avatar.png" name="Steven" />);
    expect(container.querySelector("img")).toHaveAttribute("src", "/updated-avatar.png");
  });

  it("keeps the existing silhouette fallback when the bundled image fails", () => {
    const { container, rerender } = render(<UserAvatar name="Steven" />);
    fireEvent.error(container.querySelector("img")!);
    const avatar = container.firstElementChild as HTMLElement;
    expect(avatar.style.getPropertyValue("--avatar-hue")).toMatch(/^\d+deg$/);
    expect(avatar).toHaveAttribute("aria-hidden", "true");
    expect(avatar.querySelector("svg")).toHaveAttribute("fill", "currentColor");
    const hue = avatar.style.getPropertyValue("--avatar-hue");

    rerender(<UserAvatar name="steven" />);
    expect(avatar.style.getPropertyValue("--avatar-hue")).toBe(hue);
    rerender(<UserAvatar name="Alice" />);
    expect(avatar.style.getPropertyValue("--avatar-hue")).not.toBe(hue);
    expect(avatar.querySelector("svg")).toBeInTheDocument();

    rerender(<UserAvatar />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(container.firstElementChild).toHaveClass("bg-mu-well");
  });
});

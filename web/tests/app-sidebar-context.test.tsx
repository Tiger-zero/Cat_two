import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppSidebarProvider, useAppSidebar } from "@/contexts/AppSidebarContext";

const Harness = () => {
  const navigate = useNavigate();
  const { mobileOpen, setMobileOpen, closeMobileThen, completeMobileClose } = useAppSidebar();
  const [actionCount, setActionCount] = useState(0);
  return (
    <div>
      <output data-testid="mobile-open">{String(mobileOpen)}</output>
      <output data-testid="action-count">{actionCount}</output>
      <button type="button" onClick={() => navigate("/setting")}>
        Change route
      </button>
      <button type="button" onClick={() => setMobileOpen(true)}>
        Open mobile sidebar
      </button>
      <button type="button" onClick={() => closeMobileThen(() => setActionCount((count) => count + 1))}>
        Run after close
      </button>
      <button type="button" onClick={() => completeMobileClose(false)}>
        Finish mobile close
      </button>
    </div>
  );
};

describe("AppSidebarProvider", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("closes the mobile sidebar when the route changes", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebarProvider>
          <Harness />
        </AppSidebarProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open mobile sidebar" }));
    expect(screen.getByTestId("mobile-open")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: "Change route" }));
    expect(screen.getByTestId("mobile-open")).toHaveTextContent("false");
  });

  it("defers sidebar actions until the mobile close transition finishes", () => {
    let frameCallback: FrameRequestCallback | undefined;
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback;
        return 1;
      }),
    );
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    render(
      <MemoryRouter initialEntries={["/memos/detail"]}>
        <AppSidebarProvider>
          <Harness />
        </AppSidebarProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open mobile sidebar" }));
    fireEvent.click(screen.getByRole("button", { name: "Run after close" }));
    expect(screen.getByTestId("mobile-open")).toHaveTextContent("false");
    expect(screen.getByTestId("action-count")).toHaveTextContent("0");

    fireEvent.click(screen.getByRole("button", { name: "Finish mobile close" }));
    expect(screen.getByTestId("action-count")).toHaveTextContent("0");
    act(() => frameCallback?.(0));
    expect(screen.getByTestId("action-count")).toHaveTextContent("1");

    fireEvent.click(screen.getByRole("button", { name: "Run after close" }));
    expect(screen.getByTestId("action-count")).toHaveTextContent("2");
  });

  it("cancels a pending mobile action when the route changes", () => {
    const frameCallbacks = new Map<number, FrameRequestCallback>();
    let nextFrame = 0;
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn((callback: FrameRequestCallback) => {
        nextFrame += 1;
        frameCallbacks.set(nextFrame, callback);
        return nextFrame;
      }),
    );
    vi.stubGlobal(
      "cancelAnimationFrame",
      vi.fn((frame: number) => frameCallbacks.delete(frame)),
    );

    render(
      <MemoryRouter initialEntries={["/memos/detail"]}>
        <AppSidebarProvider>
          <Harness />
        </AppSidebarProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open mobile sidebar" }));
    fireEvent.click(screen.getByRole("button", { name: "Run after close" }));
    fireEvent.click(screen.getByRole("button", { name: "Change route" }));
    fireEvent.click(screen.getByRole("button", { name: "Finish mobile close" }));
    act(() => frameCallbacks.forEach((callback) => callback(0)));

    expect(screen.getByTestId("action-count")).toHaveTextContent("0");
  });
});

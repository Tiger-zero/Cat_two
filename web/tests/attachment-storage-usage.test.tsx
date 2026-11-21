import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import UserStatsSection from "@/components/Settings/UserStatsSection";
import { userKeys } from "@/hooks/useUserQueries";

const mocks = vi.hoisted(() => ({
  getUserStats: vi.fn(),
  user: { name: "users/alice" } as { name: string } | undefined,
}));

vi.mock("@/connect", () => ({ userServiceClient: mocks }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => mocks.user }));
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) =>
    ({
      "common.statistics": "Statistics",
      "setting.account.attachment-storage": "Attachment storage",
      "setting.account.storage-usage-error": "Your attachment storage is unavailable.",
    })[key] ?? key,
}));

const setup = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  return { client, wrapper };
};

describe("account statistics", () => {
  beforeEach(() => {
    mocks.user = { name: "users/alice" };
    mocks.getUserStats.mockReset();
  });

  it("distinguishes loading, zero bytes, and a failed read", async () => {
    let resolve!: (value: { attachmentStorageBytes: bigint }) => void;
    mocks.getUserStats.mockReturnValueOnce(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const { client, wrapper } = setup();
    render(<UserStatsSection />, { wrapper });
    expect(screen.getByRole("heading", { name: "Statistics" })).toBeInTheDocument();
    expect(screen.getByText("Attachment storage")).toBeInTheDocument();
    expect(screen.getByText("…")).toBeInTheDocument();
    await act(async () => resolve({ attachmentStorageBytes: 0n }));
    expect(await screen.findByText("0 B")).toBeInTheDocument();

    mocks.getUserStats.mockRejectedValue(new Error("offline"));
    await act(async () => {
      await client.invalidateQueries({ queryKey: userKeys.stats() });
    });
    expect(await screen.findByText("Your attachment storage is unavailable.")).toBeInTheDocument();
    expect(screen.queryByText("0 B")).not.toBeInTheDocument();
  });

  it("does not show another user's cached total after an account change", async () => {
    mocks.getUserStats.mockResolvedValueOnce({ attachmentStorageBytes: 1024n }).mockResolvedValueOnce({ attachmentStorageBytes: 2048n });
    const { wrapper } = setup();
    const { rerender } = render(<UserStatsSection />, { wrapper });
    expect(await screen.findByText("1.0 KB")).toBeInTheDocument();
    mocks.user = { name: "users/bob" };
    rerender(<UserStatsSection />);
    expect(screen.queryByText("1.0 KB")).not.toBeInTheDocument();
    expect(await screen.findByText("2.0 KB")).toBeInTheDocument();
  });

  it("does not request or display usage without a signed-in user", () => {
    mocks.user = undefined;
    const { wrapper } = setup();
    const { container } = render(<UserStatsSection />, { wrapper });
    expect(container).toBeEmptyDOMElement();
    expect(mocks.getUserStats).not.toHaveBeenCalled();
  });
});

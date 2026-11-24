import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { authenticatedFetch } from "@/connect";

interface LoginBlocklistPage {
  entries: { ip: string; blockedAt: string }[];
  nextPageToken?: string;
}

const loginBlocklistPath = "/internal/auth/loginBlocklist";

export function useLoginBlocklist(account: string | undefined) {
  const queryClient = useQueryClient();
  const queryKey = ["login-blocklist", account] as const;
  const query = useInfiniteQuery({
    queryKey,
    enabled: Boolean(account),
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }): Promise<LoginBlocklistPage> => {
      const query = pageParam ? `?${new URLSearchParams({ pageToken: pageParam })}` : "";
      const response = await authenticatedFetch(loginBlocklistPath + query, { signal, cache: "no-store" });
      if (!response.ok) throw new Error(`Login blocklist request returned HTTP ${response.status}`);
      return response.json();
    },
    getNextPageParam: (page) => page.nextPageToken || undefined,
    retry: false,
  });
  const unblock = useMutation({
    mutationFn: async (ip: string) => {
      const response = await authenticatedFetch(`${loginBlocklistPath}/${encodeURIComponent(ip)}`, { method: "DELETE", cache: "no-store" });
      if (!response.ok) throw new Error(`Login unblock request returned HTTP ${response.status}`);
    },
    onSuccess: async (_response, ip) => {
      // Drop the entry immediately after the server confirms the change. A
      // failed refresh must not make the completed unblock look unsuccessful.
      queryClient.setQueryData<typeof query.data>(
        queryKey,
        (data) =>
          data && { ...data, pages: data.pages.map((page) => ({ ...page, entries: page.entries.filter((entry) => entry.ip !== ip) })) },
      );
      await queryClient.invalidateQueries({ queryKey, exact: true });
    },
  });
  return { query, unblock };
}

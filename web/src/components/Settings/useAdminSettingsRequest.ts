import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { authenticatedFetch } from "@/connect";
import useCurrentUser from "@/hooks/useCurrentUser";

export default function useAdminSettingsRequest<Settings, Update>(path: string) {
  const account = useCurrentUser()?.name;
  const queryClient = useQueryClient();
  const queryKey = ["admin-settings", path, account] as const;
  const savingRef = useRef(false);
  const [saving, setSaving] = useState(false);
  const query = useQuery({
    queryKey,
    enabled: Boolean(account),
    retry: false,
    queryFn: async ({ signal }): Promise<Settings> => {
      const response = await authenticatedFetch(path, { signal, cache: "no-store" });
      if (!response.ok) throw new Error("Could not load settings");
      return response.json();
    },
  });

  // Write-only credentials stay out of the React Query mutation cache.
  const save = async (update: Update) => {
    if (savingRef.current) throw new Error("A settings update is in progress");
    savingRef.current = true;
    setSaving(true);
    try {
      const response = await authenticatedFetch(path, {
        method: "PATCH",
        cache: "no-store",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(update),
      });
      if (!response.ok) throw new Error("Could not save settings");
      const settings: Settings = await response.json();
      await queryClient.cancelQueries({ queryKey, exact: true });
      queryClient.setQueryData(queryKey, settings);
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return { query, saving, save };
}

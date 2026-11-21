import { useQuery } from "@tanstack/react-query";
import { authenticatedFetch } from "@/connect";

interface TwoFactorStatus {
  enabled: boolean;
}

interface Proof {
  password: string;
  code?: string;
}

export async function requestTwoFactor<T>(path: "" | "/setup" | "/enable" | "/disable", proof?: Proof, signal?: AbortSignal): Promise<T> {
  const response = await authenticatedFetch(`/api/auth/totp${path}`, {
    signal,
    cache: "no-store",
    ...(proof && { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(proof) }),
  });
  if (!response.ok) throw new Error(`Two-factor request returned ${response.status}`);
  return response.json() as Promise<T>;
}

export function useTwoFactorStatus(userName: string | undefined) {
  return useQuery({
    queryKey: ["account-two-factor", userName],
    queryFn: ({ signal }) => requestTwoFactor<TwoFactorStatus>("", undefined, signal),
    enabled: Boolean(userName),
    retry: false,
  });
}

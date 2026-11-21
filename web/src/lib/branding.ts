export const DEFAULT_FAVICON = "/favicon.png?v=cat-two";
export const DEFAULT_LOGO = "/logo.webp?v=cat-two";

// Older profiles can contain a built-in asset URL instead of inheriting the default.
export const getCustomLogoUrl = (logoUrl?: string): string | undefined =>
  !logoUrl || logoUrl === "/logo.webp" || logoUrl === "/logo.svg" || logoUrl === "/logo-small.svg" ? undefined : logoUrl;

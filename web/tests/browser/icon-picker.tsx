import { Suspense, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import CustomIconPicker from "@/components/CustomIconPicker";
import i18n from "@/i18n";
import type { CustomIconValue } from "@/lib/custom-icons";
import "@/index.css";

declare global {
  interface Window {
    iconPickerTest: {
      setCustom: (custom: boolean) => void;
      setLocale: (locale: string) => Promise<void>;
      changes: (string | null)[];
    };
  }
}

const changes: (string | null)[] = [];

function Harness() {
  const [value, setValue] = useState<CustomIconValue>();
  useEffect(() => {
    window.iconPickerTest = {
      setCustom: (custom) => setValue(custom ? { value: { case: "lucide", value: "leaf" } } : undefined),
      setLocale: async (locale) => {
        await i18n.changeLanguage(locale);
        document.documentElement.lang = locale;
      },
      changes,
    };
  }, []);

  return (
    <main className="flex justify-center pt-24">
      <CustomIconPicker
        label="Choose icon"
        value={value}
        onChange={(next) => {
          changes.push(next?.value.value ?? null);
          setValue(next);
        }}
      />
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <Suspense>
    <Harness />
  </Suspense>,
);

import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import english from "./locales/en.json";
import simplifiedChinese from "./locales/zh-Hans.json";

export const DEFAULT_LOCALE = "zh-Hans";
export const locales = [DEFAULT_LOCALE, "en"];

i18n.use(initReactI18next).init({
  lng: DEFAULT_LOCALE,
  supportedLngs: locales,
  load: "currentOnly",
  fallbackLng: "en",
  resources: {
    "zh-Hans": { translation: simplifiedChinese },
    en: { translation: english },
  },
  interpolation: { escapeValue: false },
});

export default i18n;

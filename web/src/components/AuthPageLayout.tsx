import { ArrowLeftIcon } from "lucide-react";
import { Link } from "react-router-dom";
import { cn } from "@/lib/utils";
import { ROUTES } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";
import AuthFooter from "./AuthFooter";
import MemosLogo from "./MemosLogo";

interface Props {
  title?: string;
  description?: string;
  children: React.ReactNode;
}

// The way back from administrator sign-in.
export const AuthBackLink = ({ to = ROUTES.AUTH }: { to?: string }) => {
  const t = useTranslate();
  return (
    <p className="mt-5 text-center mu-caption">
      <Link to={to} className="inline-flex items-center gap-1 text-mu-accent hover:underline" viewTransition>
        <ArrowLeftIcon className="mu-icon-sm rtl:rotate-180" />
        {t("auth.back-to-sign-in")}
      </Link>
    </p>
  );
};

/**
 * Every auth page: the instance's identity, the task's heading, the task, and a footer row with the
 * visitor's language and theme. A card on wider screens; on a phone the frame drops and the footer
 * sits at the bottom of the screen.
 */
const AuthPageLayout = ({ title, description, children }: Props) => (
  <div className="flex min-h-svh w-full flex-col sm:items-center sm:justify-center sm:p-8">
    <main className="flex w-full flex-1 flex-col sm:memos-panel sm:w-90 sm:flex-none">
      <div className="flex flex-1 flex-col justify-center px-5 py-12 sm:px-7 sm:pt-8 sm:pb-7">
        <MemosLogo size="lg" className="mb-8" />
        {title && <h1 className="mu-heading text-mu-text">{title}</h1>}
        {description && <p className="mt-1 mu-body text-mu-text-secondary">{description}</p>}
        <div className={cn("w-full", title && "mt-6")}>{children}</div>
      </div>
      {/* Same side padding as the content above, so the footer's controls share its edges. */}
      <AuthFooter className="border-t border-mu-line px-5 pb-[max(0.5rem,env(safe-area-inset-bottom))] sm:rounded-b-mu-xl sm:bg-mu-canvas/60 sm:px-7 sm:pb-2" />
    </main>
  </div>
);

export default AuthPageLayout;

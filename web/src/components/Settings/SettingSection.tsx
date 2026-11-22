import React from "react";
import { cn } from "@/lib/utils";

interface SettingSectionProps {
  title?: React.ReactNode;
  description?: string;
  children: React.ReactNode;
  className?: string;
  actions?: React.ReactNode;
}

const SettingSection: React.FC<SettingSectionProps> = ({ title, description, children, className, actions }) => {
  return (
    <div className={cn("mx-auto flex w-full max-w-3xl min-w-0 flex-col gap-6 pb-2 sm:gap-7", className)}>
      {(title || description || actions) && (
        <header className="flex min-w-0 flex-col gap-3 border-b border-mu-line/70 pb-4 sm:flex-row sm:items-start sm:justify-between sm:pb-5">
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            {title && <div className="mu-heading text-mu-text">{typeof title === "string" ? <h1>{title}</h1> : title}</div>}
            {description && <p className="max-w-[65ch] mu-body text-mu-text-secondary">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className="flex min-w-0 flex-col gap-6">{children}</div>
    </div>
  );
};

export default SettingSection;

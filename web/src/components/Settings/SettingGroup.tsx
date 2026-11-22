import React, { useId } from "react";
import { cn } from "@/lib/utils";
import { SettingGroupTitleContext } from "./SettingList";

interface SettingGroupProps {
  title?: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
  showSeparator?: boolean;
  actions?: React.ReactNode;
}

const SettingGroup: React.FC<SettingGroupProps> = ({ title, description, children, className, showSeparator = false, actions }) => {
  const titleId = useId();
  return (
    <div className={cn("flex min-w-0 flex-col gap-3", showSeparator && "border-t border-mu-line/75 pt-6", className)}>
      {(title || description || actions) && (
        <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          {(title || description) && (
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              {title && (
                <h2 id={titleId} className="mu-label text-mu-text">
                  {title}
                </h2>
              )}
              {description && <p className="max-w-[65ch] mu-caption text-mu-text-secondary">{description}</p>}
            </div>
          )}
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-2 sm:ms-auto">{actions}</div>}
        </div>
      )}
      <SettingGroupTitleContext.Provider value={title ? titleId : undefined}>
        <div className="flex min-w-0 flex-col gap-3">{children}</div>
      </SettingGroupTitleContext.Provider>
    </div>
  );
};

export default SettingGroup;

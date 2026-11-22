import type { CollectionSummary } from "@/hooks/useFilteredMemoStats";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

interface Props {
  summary?: CollectionSummary;
  loading: boolean;
  desktop: boolean;
}

const CollectionStats = ({ summary, loading, desktop }: Props) => {
  const t = useTranslate();
  const metrics = [
    { label: t("common.notes"), value: summary?.memoCount },
    { label: t("common.tags"), value: summary?.tagCount },
    { label: t("common.recorded-days"), value: summary?.recordedDays },
  ];

  return (
    <section aria-label={t("common.statistics")} aria-busy={loading}>
      <dl
        className={cn(
          "grid grid-cols-3 divide-x divide-mu-line/70",
          desktop
            ? "border-b border-mu-line/65 pt-1 pb-4"
            : "rounded-mu-lg border border-mu-line/70 bg-mu-surface py-2.5 dark:bg-mu-canvas",
        )}
      >
        {metrics.map(({ label, value }) => (
          <div key={label} className={cn("flex min-w-0 flex-col gap-0.5 px-2.5", desktop && "gap-1 first:ps-2")}>
            <dt className="mu-caption text-mu-text-secondary">{label}</dt>
            <dd
              className={cn(
                "text-mu-text tabular-nums wrap-anywhere",
                desktop ? "-order-1 mu-heading font-semibold @min-[284px]/sidebar:text-2xl @min-[284px]/sidebar:leading-tight" : "mu-label",
              )}
            >
              {value === undefined ? "—" : value.toLocaleString()}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
};

export default CollectionStats;

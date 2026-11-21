import type { CollectionSummary } from "@/hooks/useFilteredMemoStats";
import { useTranslate } from "@/utils/i18n";

interface Props {
  summary?: CollectionSummary;
  loading: boolean;
}

const CollectionStats = ({ summary, loading }: Props) => {
  const t = useTranslate();
  const metrics = [
    { label: t("common.notes"), value: summary?.memoCount },
    { label: t("common.tags"), value: summary?.tagCount },
    { label: t("common.recorded-days"), value: summary?.recordedDays },
  ];

  return (
    <section aria-label={t("common.statistics")} aria-busy={loading}>
      <dl className="grid grid-cols-3 divide-x divide-mu-line/70 rounded-mu-lg border border-mu-line/70 bg-mu-surface py-2.5 dark:bg-mu-canvas">
        {metrics.map(({ label, value }) => (
          <div key={label} className="flex min-w-0 flex-col gap-0.5 px-2.5">
            <dt className="mu-caption text-mu-text-secondary">{label}</dt>
            <dd className="mu-label text-mu-text tabular-nums wrap-anywhere">{value === undefined ? "—" : value.toLocaleString()}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
};

export default CollectionStats;

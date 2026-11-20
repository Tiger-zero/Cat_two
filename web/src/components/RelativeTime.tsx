import type { Format } from "@github/relative-time-element";
import i18n from "@/i18n";
import { APP_TIME_ZONE } from "@/lib/time-zone";

interface RelativeTimeProps {
  date?: Date;
  format?: Format;
}

const RelativeTime = ({ date, format }: RelativeTimeProps) => (
  <relative-time datetime={date?.toISOString()} lang={i18n.language} format={format} time-zone={APP_TIME_ZONE} no-title=""></relative-time>
);

export default RelativeTime;

import type { ComponentPropsWithRef, ReactNode } from "react";

export type RavenSeedIconProps = Omit<
  ComponentPropsWithRef<"svg">,
  "children"
> & {
  size?: number | string;
  title?: string;
};

function IconBase({
  size = 20,
  title,
  width,
  height,
  role,
  ref,
  children,
  "aria-label": ariaLabel,
  "aria-hidden": ariaHidden,
  ...svgProps
}: RavenSeedIconProps & { children: ReactNode }) {
  const accessible = Boolean(title || ariaLabel);
  return (
    <svg
      {...svgProps}
      ref={ref}
      viewBox="0 0 24 24"
      width={width ?? size}
      height={height ?? size}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      focusable="false"
      role={role ?? (accessible ? "img" : undefined)}
      aria-label={ariaLabel}
      aria-hidden={ariaHidden ?? (accessible ? undefined : true)}
    >
      {title ? <title>{title}</title> : null}
      {children}
    </svg>
  );
}

export function CloseIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="m6 6 12 12M18 6 6 18" />
    </IconBase>
  );
}

export function EyeIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
    </IconBase>
  );
}

export function EyeOffIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M3 3l18 18M9.8 5.3A10 10 0 0 1 12 5c6.5 0 10 7 10 7a20 20 0 0 1-3 4M6.2 6.2C3.5 8.3 2 12 2 12s3.5 7 10 7a11 11 0 0 0 5.8-1.8M10 10a3 3 0 0 0 4 4" />
    </IconBase>
  );
}

export function SearchIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="10.25" cy="10.25" r="5.75" />
      <path d="M14.55 14.55 20 20" />
    </IconBase>
  );
}
export function SearchHistoryIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 7.5v4.9l3.35 2.05" />
    </IconBase>
  );
}
export function UploadIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 14V4m0 0 4 4m-4-4L8 8" />
      <path d="M5 17v2h14v-2" />
    </IconBase>
  );
}
export function SettingsIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M4 6h4M11 6h9M4 12h8M15 12h5M4 18h2M9 18h11" />
      <circle cx="9.5" cy="6" r="1.5" />
      <circle cx="13.5" cy="12" r="1.5" />
      <circle cx="7.5" cy="18" r="1.5" />
    </IconBase>
  );
}
export function IndexerIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <ellipse cx="12" cy="6.3" rx="6" ry="2.6" />
      <path d="M6 6.3v10.9c0 1.45 2.7 2.6 6 2.6s6-1.15 6-2.6V6.3M6 11.1c0 1.45 2.7 2.6 6 2.6s6-1.15 6-2.6" />
    </IconBase>
  );
}
export function ExternalLinkIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M13 5h-6.2A1.8 1.8 0 0 0 5 6.8v10.4A1.8 1.8 0 0 0 6.8 19h10.4a1.8 1.8 0 0 0 1.8-1.8V11" />
      <path d="M14 5h5v5M19 5l-7 7" />
    </IconBase>
  );
}
export function ChevronRightIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="m9 6 6 6-6 6" />
    </IconBase>
  );
}
export function ChevronDownIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="m6 9 6 6 6-6" />
    </IconBase>
  );
}
export function ShieldCheckIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 3 19 6v5.3c0 4.3-2.9 7.7-7 9.7-4.1-2-7-5.4-7-9.7V6l7-3Z" />
      <path d="m9 12 2 2 4-4" />
    </IconBase>
  );
}
export function ArrowRightIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M5 12h14m0 0-5-5m5 5-5 5" />
    </IconBase>
  );
}
export function ConnectionIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="6" r="2.25" />
      <circle cx="6" cy="18" r="2.25" />
      <circle cx="18" cy="18" r="2.25" />
      <path d="m10.95 8-3.9 7.8M13.05 8l3.9 7.8M8.25 18h7.5" />
    </IconBase>
  );
}
export function CheckIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="m5.5 12.4 4.1 4.1 8.9-9" />
    </IconBase>
  );
}
export function SuccessIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="8" />
      <path d="m8.5 12.2 2.3 2.3 4.8-5" />
    </IconBase>
  );
}
export function RefreshIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M19 8V4l-2.05 2.05A7 7 0 1 0 19 14" />
      <path d="M19 4h-4" />
    </IconBase>
  );
}
export function UpgradeIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 18.5V6" />
      <path d="m6.75 11.25 5.25-5.25 5.25 5.25" />
      <path d="M5.5 20.5h13" />
    </IconBase>
  );
}
export function TargetIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="5.4" />
      <circle cx="12" cy="12" r="1.15" fill="currentColor" stroke="none" />
      <path d="M12 2.8v3M12 18.2v3M2.8 12h3M18.2 12h3" />
    </IconBase>
  );
}
export function ReleaseFamilyIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <rect x="8.25" y="3.5" width="7.5" height="4.75" rx="1.3" />
      <rect x="3.25" y="15.5" width="7.25" height="4.75" rx="1.3" />
      <rect x="13.5" y="15.5" width="7.25" height="4.75" rx="1.3" />
      <path d="M12 8.25v3.25M6.875 15.5v-4h10.25v4" />
    </IconBase>
  );
}
export function CandidateIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M11 4.5 17.5 11 11 17.5 4.5 11 11 4.5Z" />
      <circle cx="11" cy="11" r="1.65" />
      <path d="M18.5 4.5v3M17 6h3" />
    </IconBase>
  );
}
export function FilterIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M4 5h16l-6.15 7.05v5.25l-3.7 1.7v-6.95L4 5Z" />
    </IconBase>
  );
}
export function SortIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M8 5v14m0-14L5 8m3-3 3 3M16 19V5m0 14-3-3m3 3 3-3" />
    </IconBase>
  );
}
export function DownloadIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 4v10m0 0 4-4m-4 4-4-4" />
      <path d="M5 17v2h14v-2" />
    </IconBase>
  );
}
export function MatchingRulesIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="6" cy="8" r="2" />
      <circle cx="6" cy="16" r="2" />
      <path d="m5.15 8 .65.7 1.2-1.45M5.15 16l.65.7L7 15.25" />
      <path d="M10 8h9M10 16h9" />
    </IconBase>
  );
}
export function MetadataIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M4.5 5h7.1L20 13.4 13.4 20 5 11.6V5Z" />
      <circle cx="8.2" cy="8.2" r="1.35" />
    </IconBase>
  );
}
export function ErrorIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="8" />
      <path d="m9.3 9.3 5.4 5.4m0-5.4-5.4 5.4" />
    </IconBase>
  );
}
export function PendingIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 7.5v4.5l3 1.75" />
      <path d="M5.8 5.8 4.4 4.4" />
    </IconBase>
  );
}

export function MonitorIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="12" cy="12" r="8.5" />
      <circle cx="12" cy="12" r="4" />
      <path d="m12 12 5.5-5.5" />
    </IconBase>
  );
}
export function PlusIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 5v14M5 12h14" />
    </IconBase>
  );
}
export function MoreIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <circle cx="5.5" cy="12" r="0.6" fill="currentColor" />
      <circle cx="12" cy="12" r="0.6" fill="currentColor" />
      <circle cx="18.5" cy="12" r="0.6" fill="currentColor" />
    </IconBase>
  );
}

export function SpinnerIcon(props: RavenSeedIconProps) {
  return (
    <IconBase {...props}>
      <path d="M12 3.5a8.5 8.5 0 0 1 8.5 8.5" />
      <path d="M20.5 12a8.5 8.5 0 0 1-3.2 6.65" opacity=".45" />
      <path d="M17.3 18.65A8.5 8.5 0 0 1 7 19.5" opacity=".25" />
      <path d="M7 19.5A8.5 8.5 0 0 1 3.5 12" opacity=".12" />
    </IconBase>
  );
}

export function RavenMarkIcon({
  size = 24,
  color = "#fff",
  crossedEye = false,
  title,
  width,
  height,
  role,
  ref,
  "aria-label": ariaLabel,
  "aria-hidden": ariaHidden,
  ...svgProps
}: RavenSeedIconProps & { crossedEye?: boolean }) {
  const accessible = Boolean(title || ariaLabel);
  return (
    <svg
      {...svgProps}
      ref={ref}
      viewBox="0 0 128 128"
      color={color}
      width={width ?? size}
      height={height ?? size}
      fill="currentColor"
      focusable="false"
      role={role ?? (accessible ? "img" : undefined)}
      aria-label={ariaLabel}
      aria-hidden={ariaHidden ?? (accessible ? undefined : true)}
    >
      {title ? <title>{title}</title> : null}
      <path
        fillRule="evenodd"
        d="M 112 16.43 L 106.86 11.29 L 93.14 7 L 84.57 8.71 L 78.57 12.14 L 58 35.29 L 27.14 74.71 L 30.57 75.57 L 37.43 73.86 L 38.29 74.71 L 4 109 L 23.71 109 L 39.14 95.29 L 52 98.71 L 53.71 105.57 L 58.86 110.71 L 59.71 115 L 57.14 117.57 L 62.29 117.57 L 66.57 120.14 L 69.14 120.14 L 70 118.43 L 71.71 119.29 L 70.86 116.71 L 64 115 L 62.29 112.43 L 65.71 99.57 L 68.29 97 L 75.14 95.29 L 88 87.57 L 98.29 75.57 L 101.71 65.29 L 100.86 46.43 L 95.71 49.86 L 90.57 50.71 L 81.14 48.14 L 73.43 39.57 L 73.43 31 L 79.43 24.14 L 79.43 38.71 L 85.43 43.86 L 95.71 43.86 L 101.71 38.71 L 103.43 33.57 L 100 36.14 L 94 35.29 L 89.71 30.14 L 90.57 25 L 95.71 19.86 L 106.86 18.14 L 110.29 21.57 L 105.14 21.57 L 108.57 31.86 L 105.14 42.14 L 114.57 47.29 L 120.57 55 L 123.14 48.14 L 123.14 42.14 L 120.57 35.29 L 114.57 28.43 L 115.43 25 Z M 78.57 65.29 L 79.43 71.29 L 76 78.14 L 70 84.14 L 58 89.29 L 49.43 88.43 L 50.29 86.71 L 60.57 83.29 L 68.29 78.14 L 74.29 70.43 L 76 65.29 Z"
      />
      <path d="M 76 98.71 L 88.86 110.71 L 85.43 115 L 93.14 113.29 L 100.86 118.43 L 103.43 118.43 L 105.14 116.71 L 103.43 114.14 L 93.14 109.86 L 88 92.71 Z" />
      {crossedEye && (
        <path
          d="M93 22 L103 33 M103 22 L93 33"
          stroke="currentColor"
          strokeWidth={3.2}
          strokeLinecap="round"
          fill="none"
        />
      )}
    </svg>
  );
}

export const iconRegistry = {
  eye: EyeIcon,
  "eye-off": EyeOffIcon,
  close: CloseIcon,
  search: SearchIcon,
  "search-history": SearchHistoryIcon,
  upload: UploadIcon,
  settings: SettingsIcon,
  indexer: IndexerIcon,
  "external-link": ExternalLinkIcon,
  "chevron-right": ChevronRightIcon,
  "chevron-down": ChevronDownIcon,
  "shield-check": ShieldCheckIcon,
  "arrow-right": ArrowRightIcon,
  connection: ConnectionIcon,
  check: CheckIcon,
  success: SuccessIcon,
  refresh: RefreshIcon,
  target: TargetIcon,
  upgrade: UpgradeIcon,
  "release-family": ReleaseFamilyIcon,
  candidate: CandidateIcon,
  filter: FilterIcon,
  sort: SortIcon,
  download: DownloadIcon,
  "matching-rules": MatchingRulesIcon,
  metadata: MetadataIcon,
  error: ErrorIcon,
  pending: PendingIcon,
  monitor: MonitorIcon,
  plus: PlusIcon,
  more: MoreIcon,
  spinner: SpinnerIcon,
  "raven-mark": RavenMarkIcon,
} as const;

export type RavenSeedIconName = keyof typeof iconRegistry;

export const iconGallery = Object.entries(iconRegistry) as Array<
  [RavenSeedIconName, (typeof iconRegistry)[RavenSeedIconName]]
>;

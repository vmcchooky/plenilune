import type { ReactNode } from 'react';

export type IconName =
  | 'moon'
  | 'arrow'
  | 'check'
  | 'cross'
  | 'dash'
  | 'clock'
  | 'shield'
  | 'download'
  | 'copy'
  | 'file'
  | 'print'
  | 'packet'
  | 'repair'
  | 'chevron'
  | 'info'
  | 'search'
  | 'plus'
  | 'trash'
  | 'replay'
  | 'lock';

interface IconProps {
  name: IconName;
  size?: number;
  className?: string;
}

export function Icon({ name, size = 18, className }: IconProps) {
  const paths: Record<IconName, ReactNode> = {
    moon: <><circle cx="12" cy="12" r="8.5" /><path d="M12 3.5a8.5 8.5 0 0 0 0 17 6 8.5 0 0 1 0-17Z" fill="currentColor" stroke="none" /></>,
    arrow: <><path d="M4 12h15" /><path d="m13 6 6 6-6 6" /></>,
    check: <path d="m4.5 12.5 4.5 4.5L19.5 6.5" />,
    cross: <><path d="m6 6 12 12" /><path d="M18 6 6 18" /></>,
    dash: <path d="M5 12h14" />,
    clock: <><circle cx="12" cy="12" r="8.5" /><path d="M12 7v5l3.5 2" /></>,
    shield: <><path d="M12 21.5s7.5-3.6 7.5-9.5V5.5L12 2.5 4.5 5.5V12c0 5.9 7.5 9.5 7.5 9.5Z" /><path d="m8.5 12 2.5 2.5 4.5-4.5" /></>,
    download: <><path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M4.5 20.5h15" /></>,
    copy: <><rect x="8" y="8" width="11" height="12" rx="1.5" /><path d="M16 8V5.5A1.5 1.5 0 0 0 14.5 4h-9A1.5 1.5 0 0 0 4 5.5v9A1.5 1.5 0 0 0 5.5 16H8" /></>,
    file: <><path d="M14 3H6.5A1.5 1.5 0 0 0 5 4.5v15A1.5 1.5 0 0 0 6.5 21h11a1.5 1.5 0 0 0 1.5-1.5V8Z" /><path d="M14 3v5h5" /><path d="M8.5 13h7M8.5 17h5" /></>,
    print: <><path d="M7 8V3.5h10V8" /><rect x="4" y="8" width="16" height="8" rx="1.5" /><path d="M7 14h10v7H7z" /></>,
    packet: <><rect x="3" y="5" width="18" height="14" rx="1.5" /><path d="M7 9h10M7 13h5M15 13h2M7 17h3" /></>,
    repair: <><path d="m14.5 5.5 4-4a5 5 0 0 0-6.4 6.4l-6.6 6.6a2.1 2.1 0 0 0 3 3l6.6-6.6a5 5 0 0 0 6.4-6.4Z" /><path d="m15.5 15.5 5 5" /></>,
    chevron: <path d="m9 5 7 7-7 7" />,
    info: <><circle cx="12" cy="12" r="8.5" /><path d="M12 11v5.5" /><path d="M12 7.8v.2" /></>,
    search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m15.5 15.5 4.5 4.5" /></>,
    plus: <><path d="M12 5v14" /><path d="M5 12h14" /></>,
    trash: <><path d="M4.5 6.5h15" /><path d="M9 6.5V4h6v2.5" /><path d="M6.5 6.5 7.5 21h9l1-14.5" /><path d="M10 10.5v6M14 10.5v6" /></>,
    replay: <><path d="M4.5 9.5A8 8 0 1 1 4.8 15" /><path d="M4.5 4.5v5h5" /></>,
    lock: <><rect x="5" y="10" width="14" height="10.5" rx="1.5" /><path d="M8 10V7.5a4 4 0 0 1 8 0V10" /></>,
  };
  return (
    <svg
      aria-hidden="true"
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {paths[name]}
    </svg>
  );
}

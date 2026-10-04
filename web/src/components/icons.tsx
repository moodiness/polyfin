import type { ReactNode, SVGProps } from 'react'

/** Line icons drawn on a 24-unit grid with one stroke width, decorative unless labelled. */
function Icon({ children, ...props }: SVGProps<SVGSVGElement> & { children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  )
}

type IconProps = SVGProps<SVGSVGElement>

export const icons = {
  overview: (props: IconProps) => (
    <Icon {...props}>
      <path d="M3 12h4l2.5-6 5 12 2.5-6h4" />
    </Icon>
  ),
  schedule: (props: IconProps) => (
    <Icon {...props}>
      <rect x="3.5" y="5" width="17" height="15" rx="2.5" />
      <path d="M3.5 10h17M8 3v4M16 3v4M12 13.5v2.5l1.75 1" />
    </Icon>
  ),
  health: (props: IconProps) => (
    <Icon {...props}>
      <path d="M12 20s-7.5-4.6-7.5-10.2A4.3 4.3 0 0 1 12 7a4.3 4.3 0 0 1 7.5 2.8C19.5 15.4 12 20 12 20Z" />
      <path d="M7.5 12h2.25l1.25-2 2 4 1.25-2h2.25" />
    </Icon>
  ),
  logs: (props: IconProps) => (
    <Icon {...props}>
      <rect x="3.5" y="4.5" width="17" height="15" rx="2.5" />
      <path d="m7.5 9.5 2.5 2.5-2.5 2.5M12.5 15h4" />
    </Icon>
  ),
  users: (props: IconProps) => (
    <Icon {...props}>
      <circle cx="9" cy="8.5" r="3.25" />
      <path d="M3.5 19a5.5 5.5 0 0 1 11 0M15.5 5.5a3.25 3.25 0 0 1 0 6.25M17.5 14.25A5.5 5.5 0 0 1 20.5 19" />
    </Icon>
  ),
  libraries: (props: IconProps) => (
    <Icon {...props}>
      <path d="m12 4 8.5 4.25L12 12.5 3.5 8.25 12 4Z" />
      <path d="m3.5 12.25 8.5 4.25 8.5-4.25M3.5 16.25 12 20.5l8.5-4.25" />
    </Icon>
  ),
  addons: (props: IconProps) => (
    <Icon {...props}>
      <path d="M10 4.5a2 2 0 1 1 4 0V6h3a1.5 1.5 0 0 1 1.5 1.5v3H17a2 2 0 1 0 0 4h1.5v3A1.5 1.5 0 0 1 17 19h-3v-1.5a2 2 0 1 0-4 0V19H7a1.5 1.5 0 0 1-1.5-1.5v-3H7a2 2 0 1 0 0-4H5.5v-3A1.5 1.5 0 0 1 7 6h3V4.5Z" />
    </Icon>
  ),
  settings: (props: IconProps) => (
    <Icon {...props}>
      <path d="M4 7h9M17 7h3M4 17h3M11 17h9M4 12h5M13 12h7" />
      <circle cx="15" cy="7" r="2" />
      <circle cx="9" cy="17" r="2" />
      <circle cx="11" cy="12" r="2" />
    </Icon>
  ),
  key: (props: IconProps) => (
    <Icon {...props}>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 8.5-8.5M16.5 6.5l2 2M14 9l2 2" />
    </Icon>
  ),
  myAddons: (props: IconProps) => (
    <Icon {...props}>
      <rect x="4" y="4" width="7" height="7" rx="1.75" />
      <rect x="13" y="4" width="7" height="7" rx="1.75" />
      <rect x="4" y="13" width="7" height="7" rx="1.75" />
      <path d="M16.5 13.5v6M13.5 16.5h6" />
    </Icon>
  ),
  account: (props: IconProps) => (
    <Icon {...props}>
      <circle cx="12" cy="9" r="3.5" />
      <path d="M5.5 19.5a7 7 0 0 1 13 0" />
    </Icon>
  ),
  quickConnect: (props: IconProps) => (
    <Icon {...props}>
      <rect x="6.5" y="3" width="11" height="18" rx="2.5" />
      <path d="M10 7h4M9.5 12h1M12 12h.5M14 12h.5M9.5 14.5h1M12 14.5h.5M14 14.5h.5" />
    </Icon>
  ),
  external: (props: IconProps) => (
    <Icon {...props}>
      <path d="M14 4.5h5.5V10M19.5 4.5 11 13M17 14v4a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 5 18V8.5A1.5 1.5 0 0 1 6.5 7H10" />
    </Icon>
  ),
  play: (props: IconProps) => (
    <Icon {...props}>
      <path d="M8 5.5v13l10.5-6.5L8 5.5Z" />
    </Icon>
  ),
  menu: (props: IconProps) => (
    <Icon {...props}>
      <path d="M4 7h16M4 12h16M4 17h16" />
    </Icon>
  ),
  close: (props: IconProps) => (
    <Icon {...props}>
      <path d="m6 6 12 12M18 6 6 18" />
    </Icon>
  ),
  stop: (props: IconProps) => (
    <Icon {...props}>
      <rect x="6.5" y="6.5" width="11" height="11" rx="1.5" />
    </Icon>
  ),
  pause: (props: IconProps) => (
    <Icon {...props}>
      <path d="M9 6v12M15 6v12" />
    </Icon>
  ),
  message: (props: IconProps) => (
    <Icon {...props}>
      <path d="M5 5.5h14a1.5 1.5 0 0 1 1.5 1.5v8.5A1.5 1.5 0 0 1 19 17h-8l-4.5 3.5V17H5a1.5 1.5 0 0 1-1.5-1.5V7A1.5 1.5 0 0 1 5 5.5Z" />
    </Icon>
  ),
  search: (props: IconProps) => (
    <Icon {...props}>
      <circle cx="11" cy="11" r="6" />
      <path d="m20 20-4.5-4.5" />
    </Icon>
  ),
  download: (props: IconProps) => (
    <Icon {...props}>
      <path d="M12 4v11M7.5 10.5 12 15l4.5-4.5M5 19.5h14" />
    </Icon>
  ),
  check: (props: IconProps) => (
    <Icon {...props}>
      <path d="m5 12.5 4.5 4.5L19 7.5" />
    </Icon>
  ),
  music: (props: IconProps) => (
    <Icon {...props}>
      <path d="M9 18V6l10-2v12" />
      <circle cx="6.5" cy="18" r="2.5" />
      <circle cx="16.5" cy="16" r="2.5" />
    </Icon>
  ),
  alert: (props: IconProps) => (
    <Icon {...props}>
      <path d="M12 4 21 19.5H3L12 4Z" />
      <path d="M12 10v4.5M12 17h.01" />
    </Icon>
  ),
}

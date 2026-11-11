import { Link } from 'react-router'
import { languages, useI18n } from '@/i18n'
import { cx, rem, Select } from '@/ui'

/**
 * The Polyfin logo, the only place for the brand's five facets. In the top row it is the 32 px
 * logo alone, linking home; without a session (`link={false}`) it is the 40 px logo and the name,
 * which a phone leaves out.
 */
export function Brand({ link = true }: { link?: boolean }) {
  const { t } = useI18n()
  const logo = (size: number) => (
    <img
      src={`${import.meta.env.BASE_URL}polyfin.svg`}
      alt=""
      width={size}
      height={size}
      className="shrink-0"
      style={{ width: rem(size), height: rem(size) }}
    />
  )
  if (!link) {
    return (
      <span className="flex shrink-0 items-center gap-3.5 text-[1.5rem] font-[650] tracking-[-0.01em] text-ink">
        {logo(40)}
        <span className="max-md:sr-only">{t.header.productName}</span>
      </span>
    )
  }
  return (
    <Link
      to="/"
      aria-label={`${t.header.productName}, ${t.nav.home}`}
      className="focus-lit -ml-1 flex size-9 shrink-0 items-center justify-center rounded-[0.5rem] [--lift:1.06] max-[1180px]:ml-0"
    >
      {logo(32)}
    </Link>
  )
}

/** The interface's language, as a list of the languages, each named in its own language. */
export function LanguageSwitch({ className }: { className?: string }) {
  const { language, setLanguage, t } = useI18n()
  return (
    <Select
      aria-label={t.language.label}
      value={language}
      onValue={setLanguage}
      options={languages.map((code) => ({ value: code, label: t.language[code].name, lang: code }))}
      className={cx('w-[13.75rem] max-md:w-[11.5rem]', className)}
    />
  )
}

export const repositoryUrl = 'https://github.com/moodiness/polyfin'

/** The documentation's site: a page's address is this, `/`, then its file's name without `.md`. */
export const docsUrl = 'https://moodiness.github.io/polyfin/docs'

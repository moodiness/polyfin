import { Link } from 'react-router'
import { languages, useI18n } from '@/i18n'
import { cx, Select } from '@/ui'

/** The Polyfin logo and name, linking home. The logo is the only place for the brand gradient. */
export function Brand({ link = true }: { link?: boolean }) {
  const { t } = useI18n()
  const content = (
    <>
      <img
        src={`${import.meta.env.BASE_URL}polyfin.svg`}
        alt=""
        width={28}
        height={28}
        className="size-7 shrink-0"
      />
      <span>{t.header.productName}</span>
    </>
  )
  const className =
    'flex shrink-0 items-center gap-2.5 rounded-field text-[15px] font-semibold tracking-[-0.01em] text-ink'
  return link ? (
    <Link to="/" aria-label={`${t.header.productName}, ${t.nav.home}`} className={className}>
      {content}
    </Link>
  ) : (
    <span className={className}>{content}</span>
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
      className={cx('w-[184px]', className)}
    />
  )
}

export const repositoryUrl = 'https://github.com/moodiness/polyfin'

import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus } from '@/api'
import { languages, useI18n } from '@/i18n'

const repositoryUrl = 'https://github.com/moodiness/polyfin'

export default function App() {
  const { t } = useI18n()

  return (
    <div className="flex min-h-dvh flex-col">
      <div aria-hidden="true" className="bg-fin-gradient h-0.5" />
      <header className="border-b border-line">
        <div className="mx-auto flex w-full max-w-3xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <img
              src={`${import.meta.env.BASE_URL}polyfin.svg`}
              alt=""
              width={40}
              height={40}
              className="size-10 shrink-0 drop-shadow-[0_0_14px_rgb(34_211_238/0.35)]"
            />
            <div className="min-w-0">
              <p className="truncate text-lg leading-tight font-semibold tracking-tight text-white">
                {t.header.productName}
              </p>
              <p className="truncate text-sm text-muted">{t.header.subtitle}</p>
            </div>
          </div>
          <LanguageSwitch />
        </div>
      </header>

      <main className="mx-auto w-full max-w-3xl flex-1 px-4 py-8 sm:px-6 sm:py-12">
        <h1 className="text-2xl font-semibold tracking-tight text-white sm:text-3xl">
          {t.status.title}
        </h1>
        <p className="mt-2 text-muted">{t.status.description}</p>
        <div className="mt-8">
          <StatusPanel />
        </div>
      </main>

      <footer className="border-t border-line">
        <div className="mx-auto w-full max-w-3xl px-4 py-6 text-sm sm:px-6">
          <a
            href={repositoryUrl}
            rel="noreferrer"
            className="rounded-sm text-fin-5 underline decoration-fin-5/40 underline-offset-4 transition-colors hover:decoration-fin-5"
          >
            {t.footer.sourceCode}
          </a>
        </div>
      </footer>
    </div>
  )
}

function LanguageSwitch() {
  const { language, setLanguage, t } = useI18n()

  return (
    <div
      role="group"
      aria-label={t.language.label}
      className="inline-flex shrink-0 rounded-lg border border-line bg-surface p-0.5"
    >
      {languages.map((code) => (
        <button
          key={code}
          type="button"
          lang={code}
          title={t.language[code].name}
          aria-pressed={language === code}
          onClick={() => setLanguage(code)}
          className={`min-w-11 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            language === code ? 'bg-fin-2 text-white' : 'text-muted hover:text-white'
          }`}
        >
          {t.language[code].short}
        </button>
      ))}
    </div>
  )
}

function StatusPanel() {
  const { language, t } = useI18n()
  const { data, error, isFetching, dataUpdatedAt, refetch } = useQuery({
    queryKey: ['status'],
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })

  if (data === undefined && error === null) {
    return (
      <div
        role="status"
        className="rounded-2xl border border-line bg-surface p-5 motion-safe:animate-pulse"
      >
        <p className="text-muted">{t.status.loading}</p>
        <div aria-hidden="true" className="mt-5 space-y-3">
          <div className="h-4 w-1/3 rounded bg-line" />
          <div className="h-4 w-2/3 rounded bg-line" />
          <div className="h-4 w-1/4 rounded bg-line" />
        </div>
      </div>
    )
  }

  const retryButton = (
    <button
      type="button"
      onClick={() => void refetch()}
      disabled={isFetching}
      className="rounded-lg border border-line bg-ink px-4 py-2 text-sm font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-70"
    >
      {isFetching ? t.status.retrying : t.status.retry}
    </button>
  )

  if (data === undefined) {
    return (
      <div role="alert" className="rounded-2xl border border-rose-400/40 bg-rose-400/5 p-5">
        <h2 className="flex items-center gap-2 font-semibold text-rose-300">
          <span aria-hidden="true" className="size-2.5 shrink-0 rounded-full bg-rose-400" />
          {t.status.errorTitle}
        </h2>
        <p className="mt-2 text-zinc-200">{t.status.errorBody}</p>
        <div className="mt-4">{retryButton}</div>
      </div>
    )
  }

  const databaseReady = data.database === 'ready'
  const timeFormat = new Intl.DateTimeFormat(language, { timeStyle: 'medium' })

  return (
    <div className="space-y-4">
      {error !== null && (
        <div
          role="alert"
          className="flex flex-col gap-3 rounded-xl border border-amber-400/40 bg-amber-400/5 p-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <p className="text-sm text-amber-200">{t.status.staleWarning}</p>
          <div className="shrink-0">{retryButton}</div>
        </div>
      )}

      <section className="overflow-hidden rounded-2xl border border-line bg-surface">
        <dl className="divide-y divide-line">
          <StatusRow label={t.status.version}>
            <span className="font-medium text-white">{data.version}</span>
          </StatusRow>
          <StatusRow label={t.status.serverId}>
            <code className="font-mono text-sm break-all text-white select-all">
              {data.serverId}
            </code>
          </StatusRow>
          <StatusRow label={t.status.database}>
            <span
              className={`inline-flex items-center gap-2 font-medium ${
                databaseReady ? 'text-emerald-300' : 'text-amber-300'
              }`}
            >
              <span
                aria-hidden="true"
                className={`size-2.5 shrink-0 rounded-full ${
                  databaseReady ? 'bg-emerald-400' : 'bg-amber-400'
                }`}
              />
              {databaseReady ? t.status.databaseReady : t.status.databaseUnavailable}
            </span>
          </StatusRow>
        </dl>
        <div className="border-t border-line bg-ink/40 px-5 py-3 text-xs text-muted">
          <p>{t.status.updatedAt(timeFormat.format(dataUpdatedAt))}</p>
          <p className="mt-0.5">{t.status.autoRefresh}</p>
        </div>
      </section>
    </div>
  )
}

function StatusRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <dt className="text-sm text-muted">{label}</dt>
      <dd className="min-w-0 sm:text-right">{children}</dd>
    </div>
  )
}

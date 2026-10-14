import { ArrowSquareInIcon, ArrowSquareOutIcon, FileZipIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useRef, useState, type FormEvent, type ReactNode } from 'react'
import {
  ApiError,
  maxLetterboxdExportBytes,
  type LetterboxdImportStatus,
  type LetterboxdUpload,
} from '@/api'
import { errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  Field,
  Modal,
  Notice,
  Panel,
  PanelFooter,
  ProgressBar,
  RelativeTime,
  StatusPill,
  Table,
  useField,
  useToast,
  type StatusTone,
} from '@/ui'

const importTones: Record<LetterboxdImportStatus['state'], StatusTone> = {
  running: 'live',
  done: 'ok',
  failed: 'danger',
}

/** The codes that are about the file chosen, shown under it. */
const fileCodes = ['letterboxd_export_invalid', 'letterboxd_export_too_large']

/**
 * The form that sends a Letterboxd export to import into a Polyfin account: the user's own, under
 * My account, or a user's, for an administrator. The file only lives in the form's state; the
 * server reads it in memory and never keeps it. `searchable` is false when no metadata addon of the
 * server searches movies, which the form says before anything is sent. `header` comes first in the
 * form, such as the choice of the kind of import.
 */
export function LetterboxdForm({
  searchable,
  start,
  onStarted,
  header,
}: {
  searchable: boolean
  start: (upload: LetterboxdUpload) => Promise<LetterboxdImportStatus>
  onStarted: (started: LetterboxdImportStatus) => void
  header?: ReactNode
}) {
  const { t } = useI18n()
  const text = t.account.letterboxd
  const toast = useToast()
  const [file, setFile] = useState<File | null>(null)
  const [watchlist, setWatchlist] = useState(true)
  const [ratings, setRatings] = useState(true)
  const [likesAsFavorites, setLikesAsFavorites] = useState(false)
  const send = useMutation({
    mutationFn: start,
    onSuccess: (started) => {
      onStarted(started)
      setFile(null)
      toast(text.started)
    },
  })
  const tooLarge = file !== null && file.size > maxLetterboxdExportBytes
  const code = send.error instanceof ApiError ? send.error.code : null
  const message = send.isError ? errorMessage(t, send.error) : undefined
  const fileError = tooLarge
    ? text.tooLarge
    : code !== null && fileCodes.includes(code)
      ? message
      : undefined
  const otherError = send.isError && fileError === undefined ? message : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (file === null || tooLarge) return
    send.mutate({ file, watchlist, ratings, likesAsFavorites })
  }

  return (
    <form onSubmit={submit} noValidate>
      <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
        {header}
        <p className="text-small text-ink-2">{text.help}</p>
        {!searchable && <Notice tone="warn">{text.noSearch}</Notice>}
        <Field label={text.file} help={text.fileHelp} error={fileError}>
          <FilePicker
            file={file}
            onFile={(chosen) => {
              send.reset()
              setFile(chosen)
            }}
          />
        </Field>
        <p className="text-small text-ink-3">{text.watchedHelp}</p>
        <div className="flex flex-col gap-4">
          <Checkbox
            checked={watchlist}
            onChange={setWatchlist}
            label={text.watchlist}
            help={text.watchlistHelp}
          />
          <Checkbox
            checked={ratings}
            onChange={setRatings}
            label={text.ratings}
            help={text.ratingsHelp}
          />
          <Checkbox
            checked={likesAsFavorites}
            onChange={setLikesAsFavorites}
            label={text.likes}
            help={text.likesHelp}
          />
        </div>
        {otherError && (
          <Notice tone="danger" live>
            {otherError}
          </Notice>
        )}
      </div>
      <PanelFooter>
        <Button
          type="submit"
          variant="primary"
          icon={ArrowSquareInIcon}
          loading={send.isPending}
          disabled={file === null || tooLarge}
        >
          {send.isPending ? text.starting : text.start}
        </Button>
      </PanelFooter>
    </form>
  )
}

/** Chooses the export's ZIP file, and shows the one chosen with its size. */
function FilePicker({ file, onFile }: { file: File | null; onFile: (file: File) => void }) {
  const { language, t } = useI18n()
  const text = t.account.letterboxd
  const field = useField()
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        ref={input}
        id={field?.id}
        aria-describedby={field?.describedBy}
        aria-invalid={field?.invalid || undefined}
        type="file"
        accept=".zip,application/zip"
        className="sr-only"
        tabIndex={-1}
        onChange={(event) => {
          const chosen = event.target.files?.[0]
          event.target.value = ''
          if (chosen) onFile(chosen)
        }}
      />
      <Button variant="secondary" icon={FileZipIcon} onClick={() => input.current?.click()}>
        {file === null ? text.choose : text.chooseAnother}
      </Button>
      {file !== null && (
        <span className="min-w-0 text-small break-all text-ink-2">
          <span className="font-mono text-ink">{file.name}</span> ·{' '}
          {formatBytes(file.size, language)}
        </span>
      )}
    </div>
  )
}

/**
 * An import of a Letterboxd export, running or the last one: how far the search of its movies has
 * gone, then what it added, and the movies not found, with why and their Letterboxd link. `own` is
 * the user's own account, under My account.
 */
export function LetterboxdProgress({
  current,
  own,
}: {
  current: LetterboxdImportStatus
  own: boolean
}) {
  const { t } = useI18n()
  const text = t.account.letterboxd
  const [showUnmatched, setShowUnmatched] = useState(false)
  const running = current.state === 'running'
  const title = running ? text.currentTitle : own ? text.ownLastTitle : text.lastTitle
  const added = [
    text.played(current.added.played),
    ...(current.watchlist ? [text.addedToWatchlist(current.added.watchlist)] : []),
    ...(current.ratings ? [text.rated(current.added.ratings)] : []),
    ...(current.likesAsFavorites ? [text.favorites(current.added.favorites)] : []),
  ].join(', ')

  return (
    <Panel
      title={title}
      titleAs="h3"
      titleAside={
        <StatusPill tone={importTones[current.state]}>{text.states[current.state]}</StatusPill>
      }
      description={
        <>
          {text.from} · {text.startedLabel} <RelativeTime iso={current.startedAt} />
          {current.endedAt !== null && (
            <>
              {' '}
              · {text.endedLabel} <RelativeTime iso={current.endedAt} />
            </>
          )}
          {current.startedBy.id !== current.user.id && (
            <> · {text.startedBy(current.startedBy.name)}</>
          )}
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {running && <p className="text-small text-ink-3">{text.runningHelp}</p>}
        {current.problem !== null && <Notice tone="danger">{text.problem}</Notice>}
        {running ? (
          <div className="flex flex-col gap-2">
            <p className="text-body text-ink">{text.searching(current.searched, current.movies)}</p>
            <ProgressBar
              value={current.movies === 0 ? 0 : current.searched / current.movies}
              label={text.searchingLabel}
              valueText={text.searching(current.searched, current.movies)}
            />
          </div>
        ) : (
          <p className="text-body text-ink">{text.summary(current.movies, added)}</p>
        )}
        {current.unmatchedCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="self-start"
            onClick={() => setShowUnmatched(true)}
          >
            {text.unmatched(current.unmatchedCount)}
          </Button>
        )}
        <Modal
          open={showUnmatched}
          onClose={() => setShowUnmatched(false)}
          title={text.unmatchedTitle}
          width={640}
          footer={<Button onClick={() => setShowUnmatched(false)}>{t.users.close}</Button>}
        >
          <div className="space-y-4 p-5">
            <p className="text-small text-ink-3">{text.unmatchedHelp}</p>
            <Table label={text.unmatchedTitle} minWidth={480}>
              <thead>
                <tr>
                  <th scope="col">{text.movieColumn}</th>
                  <th scope="col">{text.reasonColumn}</th>
                </tr>
              </thead>
              <tbody>
                {current.unmatched.map((movie, index) => (
                  <tr key={index}>
                    <td>
                      <span className="block text-ink">
                        {movie.name}
                        {movie.year !== null && <span className="text-ink-3"> ({movie.year})</span>}
                      </span>
                      {movie.link !== null && (
                        <a
                          href={movie.link}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-small text-link hover:text-link-hover"
                        >
                          {text.onLetterboxd}
                          <ArrowSquareOutIcon size={14} aria-hidden="true" />
                        </a>
                      )}
                    </td>
                    <td>{text.reasons[movie.reason]}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
            {current.unmatchedCount > current.unmatched.length && (
              <p className="text-small text-ink-3">
                {t.users.jellyfinImport.more(current.unmatchedCount - current.unmatched.length)}
              </p>
            )}
          </div>
        </Modal>
      </div>
    </Panel>
  )
}

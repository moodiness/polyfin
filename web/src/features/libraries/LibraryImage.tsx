import {
  ImageIcon,
  PencilSimpleIcon,
  TelevisionSimpleIcon,
  TrashIcon,
  UploadSimpleIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useId, useRef, useState, type KeyboardEvent } from 'react'
import {
  ApiError,
  queryClient,
  queryKeys,
  saveLibraryImage,
  type Library,
  type LibraryImageChoice,
  type LibraryImageRequest,
  type Scope,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  cx,
  Field,
  FieldError,
  IconButton,
  Notice,
  Segmented,
  TextInput,
  useToast,
} from '@/ui'

/** The largest picture the server keeps, as jellyfin-web uploads are bounded. */
const maxImageBytes = 10 * 1024 * 1024

/** The formats the server keeps an uploaded picture in. */
const imageTypes = ['image/jpeg', 'image/png', 'image/webp']

/** The errors of a picture found at an address, placed under the address field. */
const addressErrors: Record<string, true> = {
  invalid_image_url: true,
  image_unreachable: true,
  image_private_network: true,
}

/** The image a library shows in apps, through Polyfin as apps load it; null when it shows none. */
function imageUrl(library: Library, width: number): string | null {
  if (library.itemId === null || library.imageTag === null) return null
  return `/Items/${encodeURIComponent(library.itemId)}/Images/Primary?tag=${encodeURIComponent(library.imageTag)}&maxWidth=${width}`
}

/** A library's image at 16:9, or an empty frame with an image icon when it shows none. */
function Picture({
  library,
  width,
  className,
}: {
  library: Library
  /** The width asked of the server, in pixels. */
  width: number
  className: string
}) {
  const url = imageUrl(library, width)
  const [failed, setFailed] = useState<string | null>(null)
  if (url === null || failed === url) {
    return (
      <span
        className={cx(
          'grid aspect-video place-items-center rounded-field border border-dashed border-line-3 bg-s2 text-ink-3',
          className,
        )}
      >
        <ImageIcon size={18} aria-hidden="true" />
      </span>
    )
  }
  return (
    <img
      src={url}
      alt=""
      loading="lazy"
      onError={() => setFailed(url)}
      className={cx(
        'aspect-video rounded-field border border-line-2 bg-s2 object-cover',
        className,
      )}
    />
  )
}

/** The size of the thumbnail at the start of a library's row. */
const thumbSize = 'w-14 sm:w-[72px]'

/**
 * The small 16:9 image at the start of a library's row: a button that opens or closes the
 * library's image editor below the row.
 */
export function LibraryThumb({
  id,
  library,
  name,
  open,
  controls,
  onToggle,
}: {
  id: string
  library: Library
  /** The library's name, for the button's label. */
  name: string
  open: boolean
  /** The id of the editor it opens. */
  controls: string
  onToggle: () => void
}) {
  const { t } = useI18n()
  const text = t.libraries.image
  const state =
    library.image === 'automatic' && library.imageTag === null
      ? `${text.choices.automatic}, ${text.noImage.toLocaleLowerCase()}`
      : text.choices[library.image]
  return (
    <button
      id={id}
      type="button"
      aria-expanded={open}
      aria-controls={open ? controls : undefined}
      aria-label={text.edit(name, state)}
      title={text.edit(name, state)}
      onClick={onToggle}
      className={cx(
        'group relative mt-0.5 shrink-0 cursor-pointer rounded-field transition-[box-shadow] duration-160 ease-nuit',
        open && 'shadow-[0_0_0_2px] shadow-accent/70',
        thumbSize,
      )}
    >
      <Picture library={library} width={160} className="w-full" />
      <span
        aria-hidden="true"
        className="absolute inset-0 grid place-items-center rounded-field bg-bg/60 text-ink opacity-0 transition-opacity duration-160 ease-nuit group-hover:opacity-100 group-focus-visible:opacity-100"
      >
        <PencilSimpleIcon size={16} />
      </span>
    </button>
  )
}

/** The frame of a live TV row in place of an image: its channels go to Live TV, not a tile. */
export function LiveTvThumb() {
  return (
    <span
      aria-hidden="true"
      className={cx(
        'mt-0.5 grid aspect-video shrink-0 place-items-center rounded-field border border-line bg-s2 text-ink-3',
        thumbSize,
      )}
    >
      <TelevisionSimpleIcon size={16} />
    </span>
  )
}

/** Reads a file as base64. */
async function base64Of(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  let binary = ''
  for (let start = 0; start < bytes.length; start += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(start, start + 0x8000))
  }
  return btoa(binary)
}

/**
 * Chooses the image apps show on a library's tile: none, automatic (found in its catalog) or
 * custom (uploaded, or downloaded once from an address). Each change is saved at once, apart
 * from the libraries' save bar. It opens below the library's row, from its thumbnail.
 */
export function LibraryImageEditor({
  id,
  scope,
  library,
  name,
  onClose,
}: {
  /** The id the thumbnail's `aria-controls` points to. */
  id: string
  scope: Scope
  library: Library
  name: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.libraries.image
  const toast = useToast()
  const labelId = useId()
  const fileInput = useRef<HTMLInputElement>(null)
  const section = useRef<HTMLElement>(null)
  const [choice, setChoice] = useState<LibraryImageChoice>(library.image)
  // What the server says changes the choice shown (a removal goes back to none).
  const [shown, setShown] = useState(library.image)
  if (shown !== library.image) {
    setShown(library.image)
    setChoice(library.image)
  }
  const [address, setAddress] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  // The choice that drops the uploaded image, waiting for confirmation.
  const [confirming, setConfirming] = useState<'none' | 'automatic' | null>(null)
  const key = {
    addonId: library.addonId,
    catalogType: library.catalogType,
    catalogId: library.catalogId,
  }

  const save = useMutation({
    mutationFn: (body: LibraryImageRequest) => saveLibraryImage(scope, body),
    onSuccess: (saved, body) => {
      queryClient.setQueryData(queryKeys.libraries(scope), saved)
      const removed = body.image !== 'custom' && library.image === 'custom'
      setConfirming(null)
      if ('url' in body) setAddress('')
      toast(removed ? text.removed : text.saved)
      // The remove button is gone with the image: the focus goes to the choice now made.
      if (removed) {
        requestAnimationFrame(() =>
          section.current?.querySelector<HTMLElement>('[aria-pressed="true"]')?.focus(),
        )
      }
    },
  })

  function send(body: LibraryImageRequest) {
    if (save.isPending) return
    setLocalError(null)
    save.mutate(body)
  }

  function choose(next: LibraryImageChoice) {
    if (save.isPending) return
    save.reset()
    setLocalError(null)
    if (next === 'custom') {
      setChoice(next)
    } else if (library.image === 'custom') {
      setConfirming(next)
    } else {
      setChoice(next)
      if (next !== library.image) send({ ...key, image: next })
    }
  }

  async function upload(file: File) {
    save.reset()
    if (file.size > maxImageBytes || !imageTypes.includes(file.type)) {
      setLocalError(t.errors.invalid_image)
      return
    }
    try {
      send({ ...key, image: 'custom', data: await base64Of(file) })
    } catch {
      setLocalError(t.errors.invalid_image)
    }
  }

  function applyAddress() {
    const url = address.trim()
    if (url === '') return
    save.reset()
    send({ ...key, image: 'custom', url })
  }

  const failure = save.isError && confirming === null ? save.error : null
  const atAddress =
    failure instanceof ApiError &&
    Object.hasOwn(addressErrors, failure.code) &&
    save.variables !== undefined &&
    'url' in save.variables
  const addressError = atAddress ? errorMessage(t, failure) : undefined
  const generalError =
    localError ?? (failure !== null && !atAddress ? errorMessage(t, failure) : null)
  const uploading = save.isPending && save.variables !== undefined && 'data' in save.variables
  const downloading = save.isPending && save.variables !== undefined && 'url' in save.variables
  const help =
    choice === 'automatic' && scope === 'shared'
      ? `${text.help.automatic} ${text.parental}`
      : text.help[choice]

  return (
    <section
      ref={section}
      id={id}
      aria-labelledby={labelId}
      className="animate-rise rounded-row border border-line-2 bg-s2/50 p-4 max-sm:p-3.5"
    >
      <div className="flex gap-5 max-sm:flex-col max-sm:gap-4">
        <Picture library={library} width={480} className="w-44 shrink-0 self-start sm:w-[208px]" />
        <div className="min-w-0 sm:flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 id={labelId} className="text-control font-medium text-ink">
                {text.label}
              </h3>
              <p className="mt-0.5 text-small text-ink-3">{text.atOnce}</p>
            </div>
            <IconButton size="sm" icon={XIcon} label={text.close(name)} onClick={onClose} />
          </div>
          {library.itemId === null ? (
            <Notice className="mt-4">{text.afterSave}</Notice>
          ) : (
            <div className="mt-4 space-y-4">
              <div aria-busy={save.isPending && !uploading && !downloading ? true : undefined}>
                <Segmented
                  label={text.label}
                  value={choice}
                  onChange={choose}
                  options={(['none', 'automatic', 'custom'] as const).map((value) => ({
                    value,
                    label: text.choices[value],
                  }))}
                />
                <p className="mt-2 max-w-[64ch] text-small text-ink-3">{help}</p>
                {choice === 'automatic' &&
                  library.image === 'automatic' &&
                  library.imageTag === null && (
                    <p className="mt-1.5 max-w-[64ch] text-small text-warn">{text.notFound}</p>
                  )}
              </div>
              {choice === 'custom' && (
                <>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      icon={UploadSimpleIcon}
                      loading={uploading}
                      disabled={save.isPending && !uploading}
                      onClick={() => fileInput.current?.click()}
                    >
                      {library.image === 'custom' ? text.uploadAnother : text.upload}
                    </Button>
                    <input
                      ref={fileInput}
                      type="file"
                      accept={imageTypes.join(',')}
                      tabIndex={-1}
                      aria-hidden="true"
                      className="sr-only"
                      onChange={(event) => {
                        const file = event.target.files?.[0]
                        event.target.value = ''
                        if (file !== undefined) void upload(file)
                      }}
                    />
                    {library.image === 'custom' && (
                      <Button
                        variant="danger"
                        icon={TrashIcon}
                        disabled={save.isPending}
                        onClick={() => {
                          save.reset()
                          setConfirming('none')
                        }}
                      >
                        {text.remove}
                      </Button>
                    )}
                  </div>
                  <Field label={text.address} help={text.addressHelp} error={addressError}>
                    <div className="flex gap-2 max-sm:flex-col">
                      <TextInput
                        type="url"
                        inputMode="url"
                        value={address}
                        onValue={setAddress}
                        placeholder="https://…/image.jpg"
                        autoComplete="off"
                        spellCheck={false}
                        className="min-w-0 sm:flex-1"
                        onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
                          // Enter uses the address rather than saving the libraries' form.
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            applyAddress()
                          }
                        }}
                      />
                      <Button
                        loading={downloading}
                        disabled={address.trim() === '' || (save.isPending && !downloading)}
                        onClick={applyAddress}
                      >
                        {text.useAddress}
                      </Button>
                    </div>
                  </Field>
                </>
              )}
              {generalError !== null && <FieldError>{generalError}</FieldError>}
            </div>
          )}
        </div>
      </div>
      <ConfirmDialog
        open={confirming !== null}
        onClose={() => {
          setConfirming(null)
          save.reset()
        }}
        onConfirm={() => {
          if (confirming !== null) send({ ...key, image: confirming })
        }}
        title={text.removeTitle(name)}
        confirmLabel={text.removeConfirm}
        busy={save.isPending}
        error={save.isError && confirming !== null ? errorMessage(t, save.error) : undefined}
      >
        {confirming === 'automatic' ? text.removeToAutomatic : text.removeToNone}
      </ConfirmDialog>
    </section>
  )
}

import { useMutation } from '@tanstack/react-query'
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react'
import { ApiError, updateGroup, updateUser, type GroupPatch } from '@/api'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { SaveBar, useToast } from '@/ui'
import { groupSaved, refreshGroupIfGone, refreshIfGone, userSaved } from './shared'
import type { LimitsTarget } from './UserForms'

/**
 * What a section of the page brings to the dock: how many of its settings differ from the saved
 * ones, and its part of the patch, or null while it holds an error to fix first.
 */
type DraftPart = { changes: number; patch: () => GroupPatch | null }

type Report = (id: string, part: DraftPart | null) => void

const DraftContext = createContext<Report | null>(null)

/**
 * Tells the page's dock what this section changes. Sections of a page that saves as a whole call
 * it on every render; outside a DraftForm it does nothing.
 */
export function useDraftPart(id: string, changes: number, patch: () => GroupPatch | null) {
  const report = use(DraftContext)
  const latest = useRef(patch)
  latest.current = patch
  useEffect(() => {
    report?.(id, { changes, patch: () => latest.current() })
  }, [report, id, changes])
  useEffect(() => () => report?.(id, null), [report, id])
}

/** The values a form holds against the saved ones: how many differ. */
export function countChanged<T extends object>(form: T, saved: T): number {
  return (Object.keys(form) as (keyof T)[]).filter((key) => form[key] !== saved[key]).length
}

/** The fields of a form that differ from the saved ones. */
export function changedFields<T extends object>(form: T, saved: T): Partial<T> {
  const patch: Partial<T> = {}
  for (const key of Object.keys(form) as (keyof T)[]) {
    if (form[key] !== saved[key]) patch[key] = form[key]
  }
  return patch
}

/** How many items of a and b are in one only. */
export function setDifference(a: readonly string[], b: readonly string[]): number {
  return a.filter((x) => !b.includes(x)).length + b.filter((x) => !a.includes(x)).length
}

/**
 * The form around the parts of a user's or a group's page that save as a whole, with the one dock:
 * "3 changes", Discard changes and Save, which sends every changed section in one PATCH. `version`
 * goes up after a save, a discard or a reset, for the page to draw its sections again (as their
 * `key`) from the saved values. `nameError` is the save's error when it is about the name, shown
 * under the name's row rather than in the dock.
 */
export function DraftForm({
  target,
  children,
  onVersion,
}: {
  target: LimitsTarget
  children: (state: { nameError: string | undefined }) => ReactNode
  /** Draws the sections again: after a save, or Discard changes. */
  onVersion: () => void
}) {
  const { t } = useI18n()
  const toast = useToast()
  const self = useSessionUser()
  const [counts, setCounts] = useState<Record<string, number>>({})
  const patches = useRef(new Map<string, () => GroupPatch | null>())
  const [invalid, setInvalid] = useState(false)

  const report = useCallback<Report>((id, part) => {
    if (part === null) {
      patches.current.delete(id)
      setCounts(({ [id]: _gone, ...rest }) => rest)
      return
    }
    patches.current.set(id, part.patch)
    setCounts((current) =>
      current[id] === part.changes ? current : { ...current, [id]: part.changes },
    )
  }, [])

  const changes = Object.values(counts).reduce((sum, count) => sum + count, 0)

  const save = useMutation({
    mutationFn: async (patch: GroupPatch) => {
      if (target.kind === 'group') {
        groupSaved(await updateGroup(target.group.id, patch))
      } else {
        userSaved(await updateUser(target.user.id, patch), self.id)
      }
    },
    onSuccess: () => {
      toast(target.kind === 'group' ? t.groups.updated : t.users.updated)
      onVersion()
    },
    onError: target.kind === 'group' ? refreshGroupIfGone : refreshIfGone,
  })

  // An error about the name goes under its row.
  const code = save.error instanceof ApiError ? save.error.code : null
  const aboutName = code === 'name_taken' || code === 'invalid_name'
  const nameError =
    !save.isError || !aboutName
      ? undefined
      : target.kind === 'group'
        ? code === 'name_taken'
          ? t.groups.nameTaken
          : t.groups.nameRule
        : errorMessage(t, save.error)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    // A form in a dialog opened from a section submits through the page in React's tree only.
    if (event.target !== event.currentTarget) return
    if (changes === 0 || save.isPending) return
    let patch: GroupPatch = {}
    for (const [id, part] of patches.current) {
      if ((counts[id] ?? 0) === 0) continue
      const own = part()
      if (own === null) {
        setInvalid(true)
        return
      }
      patch = { ...patch, ...own }
    }
    setInvalid(false)
    save.mutate(patch)
  }

  return (
    <form
      noValidate
      onSubmit={submit}
      onChange={() => {
        // Typing again clears the last save's error.
        setInvalid(false)
        if (save.isError) save.reset()
      }}
    >
      <DraftContext value={report}>
        <div className="[&>*+*]:mt-block">{children({ nameError })}</div>
      </DraftContext>
      <SaveBar
        dirty={changes > 0}
        saving={save.isPending}
        changes={changes}
        onDiscard={() => {
          save.reset()
          setInvalid(false)
          onVersion()
        }}
        error={
          invalid
            ? t.users.draftInvalid
            : save.isError && !aboutName
              ? errorMessage(t, save.error)
              : undefined
        }
      />
    </form>
  )
}

import { useEffect, useId, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { LockSimpleIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react'
import {
  resetTaskTriggers,
  saveTaskTriggers,
  type Task,
  type TaskTrigger,
  type TriggerType,
} from '@/api'
import { Button, Field, Modal, Notice, Select, useToast, type SelectOption } from '@/ui'
import { errorMessage, formatSpan } from '@/format'
import { useI18n, type Messages } from '@/i18n'

type Text = Messages['system']['schedule']['triggers']

/** The order the trigger types are offered in. */
const typeOrder: readonly TriggerType[] = ['daily', 'weekly', 'interval', 'startup']

/** The intervals offered, in minutes: those of Jellyfin's web app, then 2, 7, 14 and 28 days. */
const intervals = [15, 30, 45, 60, 120, 180, 240, 360, 480, 720, 1440, 2880, 10080, 20160, 40320]

/** Days of the week from Monday, 0 being Sunday. */
const weekDays = [1, 2, 3, 4, 5, 6, 0]

/** A new daily or weekly trigger starts at 03:00. */
const defaultTime = 180

/** A time of day in minutes from midnight, as "03:00" or "3:00 AM". */
function formatTime(minutes: number, language: string): string {
  return new Intl.DateTimeFormat(language, {
    hour: 'numeric',
    minute: '2-digit',
    timeZone: 'UTC',
  }).format(Date.UTC(2000, 0, 1, 0, minutes))
}

/** A day of the week by name, 0 being Sunday (2 January 2000 was one). */
function formatDay(day: number, language: string): string {
  return new Intl.DateTimeFormat(language, { weekday: 'long', timeZone: 'UTC' }).format(
    Date.UTC(2000, 0, 2 + day),
  )
}

/** "Every 6 hours": in days past one day when whole, else in hours when whole, else minutes. */
function intervalLabel(text: Text, minutes: number): string {
  if (minutes > 1440 && minutes % 1440 === 0) return text.interval(minutes / 1440, 'day')
  if (minutes % 60 === 0) return text.interval(minutes / 60, 'hour')
  return text.interval(minutes, 'minute')
}

/** A trigger in words: "Every day at 03:00", "Every Monday at 03:00", "Every 6 hours". */
export function triggerLabel(text: Text, trigger: TaskTrigger, language: string): string {
  switch (trigger.type) {
    case 'daily':
      return text.daily(formatTime(trigger.time ?? 0, language))
    case 'weekly':
      return text.weekly(
        formatDay(trigger.day ?? 0, language),
        trigger.day ?? 0,
        formatTime(trigger.time ?? 0, language),
      )
    case 'interval':
      return intervalLabel(text, trigger.interval ?? 0)
    case 'startup':
      return text.startup
  }
}

/** The trigger the add form describes. */
function newTrigger(type: TriggerType, time: number, day: number, interval: number): TaskTrigger {
  switch (type) {
    case 'daily':
      return { type, time }
    case 'weekly':
      return { type, time, day }
    case 'interval':
      return { type, interval }
    case 'startup':
      return { type }
  }
}

/**
 * A task's triggers in a dialog: each in words, with Remove, and a form to add one, saved as a
 * whole list. A trigger kept keeps its time limit, which only Jellyfin's API sets. A locked task's
 * triggers are shown read-only, with why.
 */
export function TaskTriggers({ task, onClose }: { task: Task; onClose: () => void }) {
  const { language, t } = useI18n()
  const text = t.system.schedule.triggers
  const toast = useToast()
  const client = useQueryClient()
  const limits = task.limits
  const locked = limits.locked !== ''
  const [draft, setDraft] = useState<TaskTrigger[]>(task.triggers)
  const done = (message: string) => {
    void client.invalidateQueries({ queryKey: ['tasks'] })
    toast(message, { tone: 'ok' })
    onClose()
  }
  const save = useMutation({
    mutationFn: () => saveTaskTriggers(task.id, draft),
    onSuccess: (saved) => done(text.saved(saved.name)),
  })
  const reset = useMutation({
    mutationFn: () => resetTaskTriggers(task.id),
    onSuccess: (saved) => done(text.resetDone(saved.name)),
  })
  const error = save.error ?? reset.error
  const busy = save.isPending || reset.isPending
  const changed = JSON.stringify(draft) !== JSON.stringify(task.triggers)

  // After an addition, the focus goes to the new trigger's Remove; after a removal, to the next
  // trigger's, or to the add form.
  const list = useRef<HTMLUListElement>(null)
  const addId = useId()
  const focusAt = useRef<number | null>(null)
  useEffect(() => {
    if (focusAt.current === null) return
    const buttons = list.current?.querySelectorAll<HTMLElement>('button') ?? []
    const target = buttons[Math.min(focusAt.current, buttons.length - 1)]
    focusAt.current = null
    ;(target ?? document.getElementById(addId))?.focus()
  }, [draft, addId])

  return (
    <Modal
      open
      onClose={onClose}
      title={text.title(task.name)}
      width={560}
      footer={
        locked ? (
          <Button variant="primary" onClick={onClose}>
            {t.ui.close}
          </Button>
        ) : (
          <>
            <Button
              variant="ghost"
              className="mr-auto"
              disabled={!task.custom || save.isPending}
              loading={reset.isPending}
              onClick={() => {
                save.reset()
                reset.mutate()
              }}
            >
              {text.reset}
            </Button>
            <Button variant="ghost" onClick={onClose}>
              {t.common.cancel}
            </Button>
            <Button
              variant="primary"
              disabled={!changed || (limits.keepOne && draft.length === 0) || reset.isPending}
              loading={save.isPending}
              onClick={() => {
                reset.reset()
                save.mutate()
              }}
            >
              {t.common.save}
            </Button>
          </>
        )
      }
    >
      <div className="flex flex-col gap-4 p-5">
        {limits.locked !== '' && (
          <p className="flex items-start gap-2.5 text-body text-ink-2">
            <LockSimpleIcon aria-hidden className="mt-0.5 size-5 shrink-0 text-ink-3" />
            <span>{text.locked[limits.locked]}</span>
          </p>
        )}
        {draft.length > 0 && (
          <ul ref={list} className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
            {draft.map((trigger, index) => {
              const label = triggerLabel(text, trigger, language)
              return (
                <li
                  key={`${index}-${JSON.stringify(trigger)}`}
                  className="flex min-h-12 items-center gap-3 rounded-row bg-s1 py-1.5 pr-1.5 pl-4"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-body break-words text-ink">{label}</p>
                    {trigger.maxRuntime !== undefined && (
                      <p className="text-small text-ink-3">
                        {text.stopsAfter(formatSpan(trigger.maxRuntime * 60, language))}
                      </p>
                    )}
                  </div>
                  {!locked && (
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={TrashIcon}
                      aria-label={text.removeLabel(label)}
                      disabled={busy}
                      onClick={() => {
                        setDraft(draft.filter((_, at) => at !== index))
                        focusAt.current = index
                      }}
                    >
                      {text.remove}
                    </Button>
                  )}
                </li>
              )
            })}
          </ul>
        )}
        {draft.length === 0 && !locked && (
          <Notice tone="warn" live>
            {limits.keepOne ? text.keepOne : text.none}
          </Notice>
        )}
        {draft.some((trigger) => trigger.type === 'daily' || trigger.type === 'weekly') && (
          <p className="text-small text-ink-3">{text.serverTime}</p>
        )}
        {!locked && (
          <AddTrigger
            task={task}
            draft={draft}
            disabled={busy}
            id={addId}
            onAdd={(trigger) => {
              focusAt.current = draft.length
              setDraft([...draft, trigger])
            }}
          />
        )}
        {error && (
          <Notice tone="danger" live>
            {errorMessage(t, error)}
          </Notice>
        )}
      </div>
    </Modal>
  )
}

/** The form that adds a trigger of a type the task allows, at most `limits.maxTriggers`. */
function AddTrigger({
  task,
  draft,
  disabled,
  id,
  onAdd,
}: {
  task: Task
  draft: TaskTrigger[]
  disabled: boolean
  /** The id of the type list, which the focus goes to once the last trigger is removed. */
  id: string
  onAdd: (trigger: TaskTrigger) => void
}) {
  const { language, t } = useI18n()
  const text = t.system.schedule.triggers
  const limits = task.limits
  const types = typeOrder.filter((type) => limits.types.includes(type))
  const allowed = intervals.filter(
    (minutes) => minutes >= limits.minInterval && minutes <= limits.maxInterval,
  )
  const [type, setType] = useState<TriggerType>(types[0] ?? 'interval')
  const [time, setTime] = useState(defaultTime)
  const [day, setDay] = useState(1)
  const [interval, setEvery] = useState(
    allowed.includes(1440) ? 1440 : (allowed[0] ?? limits.minInterval),
  )
  // The trigger last refused as one in the list already, while the form still describes it.
  const [refused, setRefused] = useState('')

  if (draft.length >= limits.maxTriggers) {
    return <p className="text-small text-ink-2">{text.full(limits.maxTriggers)}</p>
  }
  const trigger = newTrigger(type, time, day, interval)
  const typeOptions: SelectOption<TriggerType>[] = types.map((value) => ({
    value,
    label: text.types[value],
  }))
  const timeOptions = Array.from({ length: 96 }, (_, step) => ({
    value: step * 15,
    label: formatTime(step * 15, language),
  }))
  const dayOptions = weekDays.map((value) => ({ value, label: formatDay(value, language) }))
  const intervalOptions = allowed.map((value) => ({ value, label: intervalLabel(text, value) }))

  return (
    <section aria-labelledby={`${id}-title`} className="flex flex-col gap-3 pt-1">
      <h3 id={`${id}-title`} className="text-h3 text-ink">
        {text.add}
      </h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={text.type}>
          <Select id={id} value={type} options={typeOptions} onValue={setType} />
        </Field>
        {type === 'weekly' && (
          <Field label={text.day}>
            <Select value={day} options={dayOptions} onValue={setDay} />
          </Field>
        )}
        {(type === 'daily' || type === 'weekly') && (
          <Field label={text.time}>
            <Select value={time} options={timeOptions} onValue={setTime} />
          </Field>
        )}
        {type === 'interval' && (
          <Field label={text.intervalLabel}>
            <Select value={interval} options={intervalOptions} onValue={setEvery} />
          </Field>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Button
          icon={PlusIcon}
          disabled={disabled || (type === 'interval' && allowed.length === 0)}
          onClick={() => {
            // The same times, whatever its time limit.
            const twice = draft.some(
              (each) =>
                each.type === trigger.type &&
                each.time === trigger.time &&
                each.day === trigger.day &&
                each.interval === trigger.interval,
            )
            setRefused(twice ? JSON.stringify(trigger) : '')
            if (!twice) onAdd(trigger)
          }}
        >
          {text.add}
        </Button>
        <p role="status" className="text-small text-ink-2">
          {refused === JSON.stringify(trigger) ? text.duplicate : ''}
        </p>
      </div>
    </section>
  )
}

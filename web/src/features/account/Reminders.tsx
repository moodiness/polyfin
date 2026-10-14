import { AlarmIcon, RepeatIcon, TrashIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  deleteOwnReminder,
  fetchNotifications,
  fetchOwnReminders,
  fetchReminderSettings,
  queryClient,
  queryKeys,
  updateReminderSettings,
  type ReminderSettings,
} from '@/api'
import { dateTime, errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  EmptyState,
  Field,
  InlineError,
  Notice,
  Row,
  RowList,
  Select,
  SkeletonRows,
  useToast,
} from '@/ui'

/**
 * My account › Reminders: the programmes the user asked to be reminded of from the web player,
 * then the series reminders, each with its channel, its start and Delete.
 */
export function ReminderList() {
  const { language, t } = useI18n()
  const text = t.account.reminders
  const toast = useToast()
  const reminders = useQuery({
    queryKey: queryKeys.ownReminders,
    queryFn: ({ signal }) => fetchOwnReminders(signal),
  })
  const removal = useMutation({
    mutationFn: ({ id }: { id: string; name: string }) => deleteOwnReminder(id),
    onSuccess: (_data, { name }) => toast(text.deleted(name)),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownReminders }),
  })

  if (reminders.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (reminders.isError) {
    return (
      <InlineError onRetry={() => void reminders.refetch()} retrying={reminders.isFetching}>
        {errorMessage(t, reminders.error)}
      </InlineError>
    )
  }
  const { reminders: single, series } = reminders.data
  const timeOfDay = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  const deleteButton = (id: string, name: string) => (
    <Button
      variant="ghost"
      size="sm"
      icon={TrashIcon}
      aria-label={text.deleteLabel(name)}
      loading={removal.isPending && removal.variables.id === id}
      onClick={() => removal.mutate({ id, name })}
    >
      {text.delete}
    </Button>
  )
  return (
    <div className="space-y-3">
      {removal.isError && <Notice tone="danger">{errorMessage(t, removal.error)}</Notice>}
      {single.length === 0 && series.length === 0 ? (
        <EmptyState icon={AlarmIcon} title={text.empty}>
          {text.emptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.account.sections.reminders}>
          {single.map((reminder) => (
            <Row
              key={reminder.id}
              leading={<AlarmIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />}
              title={reminder.name}
              meta={
                <span className="truncate">
                  {reminder.channelName} · {dateTime(reminder.start, language)}
                </span>
              }
              trailing={deleteButton(reminder.id, reminder.name)}
            />
          ))}
          {series.map((reminder) => (
            <Row
              key={reminder.id}
              leading={<RepeatIcon size={18} aria-hidden="true" className="shrink-0 text-ink-3" />}
              title={reminder.name}
              meta={
                <span className="truncate">
                  {reminder.channelName} ·{' '}
                  {text.everyAiring(timeOfDay.format(new Date(reminder.start)))} ·{' '}
                  {reminder.next === null
                    ? text.noAiring
                    : `${text.next}${dateTime(reminder.next, language)}`}
                </span>
              }
              trailing={deleteButton(reminder.id, reminder.name)}
            />
          ))}
        </RowList>
      )}
    </div>
  )
}

/**
 * The reminder settings, under My account › Notifications: Remind me before, and Also show
 * reminders on my apps, each saved as it changes. It says so when no reminder would reach the user:
 * no target of theirs, turned on, chose Programme starting, and the apps are off. Nothing shows for a
 * user without the Live TV permission.
 */
export function ReminderSettingsForm() {
  const { t } = useI18n()
  const text = t.account.reminders
  const toast = useToast()
  const settings = useQuery({
    queryKey: queryKeys.reminderSettings,
    queryFn: ({ signal }) => fetchReminderSettings(signal),
  })
  // The same query as the targets above, which it follows as they change.
  const targets = useQuery({
    queryKey: queryKeys.notifications('own'),
    queryFn: ({ signal }) => fetchNotifications('own', signal),
  })
  const saving = useMutation({
    mutationFn: updateReminderSettings,
    onSuccess: (saved) => {
      queryClient.setQueryData<ReminderSettings>(queryKeys.reminderSettings, saved)
      toast(text.saved)
    },
  })

  if (settings.isPending) return null
  if (settings.isError) {
    return (
      <InlineError onRetry={() => void settings.refetch()} retrying={settings.isFetching}>
        {errorMessage(t, settings.error)}
      </InlineError>
    )
  }
  if (!settings.data.available) return null
  const { beforeMinutes, onApps, choices } = settings.data
  const reaches =
    onApps ||
    targets.data === undefined ||
    targets.data.targets.some(
      (target) => target.enabled && target.events.includes('programme_reminder'),
    )
  return (
    <div className="space-y-4 rounded-panel border border-line-2 bg-s1 px-[22px] py-5 max-sm:px-4">
      <h3 className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
        {text.settingsTitle}
      </h3>
      <Field label={text.before} help={text.beforeHelp}>
        <Select
          value={beforeMinutes}
          options={choices.map((minutes) => ({ value: minutes, label: text.minutes(minutes) }))}
          onValue={(minutes) => saving.mutate({ beforeMinutes: minutes, onApps })}
          disabled={saving.isPending}
          className="max-w-xs"
        />
      </Field>
      <Checkbox
        label={text.onApps}
        help={text.onAppsHelp}
        checked={onApps}
        disabled={saving.isPending}
        onChange={(checked) => saving.mutate({ beforeMinutes, onApps: checked })}
      />
      {!reaches && <Notice tone="warn">{text.noTarget}</Notice>}
      {saving.isError && <Notice tone="danger">{errorMessage(t, saving.error)}</Notice>}
    </div>
  )
}

import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchOwnWebTheme,
  queryClient,
  queryKeys,
  saveOwnWebTheme,
  type OwnWebTheme as OwnWebThemeAnswer,
  type UserWebTheme,
  type WebThemes,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Field, InlineError, Notice, Select, SkeletonRows, useToast } from '@/ui'

/**
 * A choice of web player theme: The server's, naming it, then the themes offered. A theme no longer
 * offered, such as Custom once its texts are emptied, stays listed while it is the one chosen.
 */
export function WebThemeSelect({
  value,
  themes,
  onValue,
  disabled,
  'aria-describedby': describedBy,
}: {
  value: UserWebTheme
  themes: WebThemes
  onValue: (theme: UserWebTheme) => void
  disabled: boolean
  'aria-describedby'?: string
}) {
  const { t } = useI18n()
  const text = t.account.webTheme
  const offered =
    value === '' || themes.themes.includes(value) ? themes.themes : [...themes.themes, value]
  return (
    <Select<UserWebTheme>
      value={value}
      options={[
        { value: '', label: text.server(text.choices[themes.serverTheme]) },
        ...offered.map((theme) => ({ value: theme, label: text.choices[theme] })),
      ]}
      onValue={onValue}
      disabled={disabled}
      aria-describedby={describedBy}
      className="max-w-xs"
    />
  )
}

/**
 * My account › Web player theme: the theme the user's web player shows on every browser, saved as
 * it changes; read only, with why, while the administrator keeps them from changing it.
 */
export function OwnWebTheme() {
  const { t } = useI18n()
  const text = t.account.webTheme
  const toast = useToast()
  const current = useQuery({
    queryKey: queryKeys.ownWebTheme,
    queryFn: ({ signal }) => fetchOwnWebTheme(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnWebTheme,
    onSuccess: (saved) => {
      queryClient.setQueryData<OwnWebThemeAnswer>(queryKeys.ownWebTheme, saved)
      // An administrator's own page under Users shows their theme.
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(text.saved)
    },
  })

  if (current.isPending) return <SkeletonRows rows={1} boxed label={t.common.loading} />
  if (current.isError) {
    return (
      <InlineError onRetry={() => void current.refetch()} retrying={current.isFetching}>
        {errorMessage(t, current.error)}
      </InlineError>
    )
  }
  const { webTheme, locked } = current.data
  return (
    <div className="space-y-4">
      <Field
        label={text.label}
        hideLabel
        error={save.isError ? errorMessage(t, save.error) : undefined}
      >
        <WebThemeSelect
          value={webTheme}
          themes={current.data}
          onValue={(chosen) => save.mutate(chosen)}
          disabled={locked || save.isPending}
        />
      </Field>
      {locked && <Notice>{text.locked}</Notice>}
    </div>
  )
}

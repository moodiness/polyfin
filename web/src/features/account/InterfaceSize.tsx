import { useMutation } from '@tanstack/react-query'
import { queryClient, queryKeys, saveOwnInterfaceSize, type SessionUser } from '@/api'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { keepInterfaceSize, type InterfaceSize } from '@/interfaceSize'
import { SettingRow, useToast } from '@/ui'

/**
 * My account › Interface size: Compact or Large, a setting row saved as it changes. The session
 * holds it, so the page shows it at once. The size saved applies at once, the choice under the
 * keyboard or the mouse kept where it was on the screen while the page grows or shrinks around it.
 */
export function OwnInterfaceSize() {
  const { t } = useI18n()
  const text = t.account.interfaceSize
  const toast = useToast()
  const user = useSessionUser()
  const save = useMutation({
    mutationFn: saveOwnInterfaceSize,
    onSuccess: ({ interfaceSize }) => {
      const chosen = document.activeElement
      const before = chosen?.getBoundingClientRect().top
      keepInterfaceSize(interfaceSize)
      if (chosen && before !== undefined) {
        window.scrollBy({ top: chosen.getBoundingClientRect().top - before, behavior: 'instant' })
      }
      queryClient.setQueryData<SessionUser | null>(
        queryKeys.session,
        (current) => current && { ...current, interfaceSize },
      )
      toast(text.saved)
    },
  })
  return (
    <SettingRow<InterfaceSize>
      kind="choice"
      label={text.label}
      help={text.help}
      value={user.interfaceSize}
      options={[
        { value: 'compact', label: text.compact },
        { value: 'large', label: text.large },
      ]}
      onChange={(chosen) => save.mutate(chosen)}
      disabled={save.isPending}
      error={save.isError ? errorMessage(t, save.error) : undefined}
    />
  )
}

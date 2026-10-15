import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchOwnMetadataLanguage,
  queryClient,
  queryKeys,
  saveOwnMetadataLanguage,
  type MetadataLanguage,
} from '@/api'
import { errorMessage, languageOptions } from '@/format'
import { useI18n } from '@/i18n'
import { Field, InlineError, Notice, Select, SkeletonRows, useToast } from '@/ui'

/**
 * A choice of metadata language: The server's, then the languages the server's addons were given,
 * by name. A language no addon has any more stays listed while it is the one chosen.
 */
export function MetadataLanguageSelect({
  value,
  languages,
  onValue,
  disabled,
}: {
  value: string
  languages: readonly string[]
  onValue: (language: string) => void
  disabled: boolean
}) {
  const { language, t } = useI18n()
  const codes = value === '' || languages.includes(value) ? languages : [...languages, value]
  return (
    <Select
      value={value}
      options={[
        { value: '', label: t.account.metadataLanguage.server },
        ...languageOptions(codes, language),
      ]}
      onValue={onValue}
      disabled={disabled}
      className="max-w-xs"
    />
  )
}

/**
 * My account › Metadata language: the language the user's titles are described in, saved as it
 * changes. Without a language given to any server addon it says where an administrator gives one.
 */
export function OwnMetadataLanguage() {
  const { t } = useI18n()
  const text = t.account.metadataLanguage
  const toast = useToast()
  const current = useQuery({
    queryKey: queryKeys.ownMetadataLanguage,
    queryFn: ({ signal }) => fetchOwnMetadataLanguage(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnMetadataLanguage,
    onSuccess: (saved) => {
      queryClient.setQueryData<MetadataLanguage>(queryKeys.ownMetadataLanguage, saved)
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
  const { language, languages } = current.data
  return (
    <div className="space-y-4">
      <Field
        label={text.label}
        hideLabel
        error={save.isError ? errorMessage(t, save.error) : undefined}
      >
        <MetadataLanguageSelect
          value={language}
          languages={languages}
          onValue={(chosen) => save.mutate(chosen)}
          disabled={save.isPending}
        />
      </Field>
      {languages.length === 0 && language === '' && <Notice>{text.noLanguage}</Notice>}
    </div>
  )
}

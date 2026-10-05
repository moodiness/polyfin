import { RecordIcon } from '@phosphor-icons/react'
import { settingsPath } from '@/app/navigation'
import LibraryEditor from '@/components/LibraryEditor'
import { PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'
import { ButtonLink } from '@/ui'

/**
 * `/live-tv`: TV catalogs with their guides, and recordings.
 * Temporary: before « Nuit » TV catalogs were rows of the library editor, where each opens its
 * guides; this shows that editor, with a link to the recording settings, until the Live TV area
 * builds the page.
 */
export default function LiveTvRoute() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader
        title={t.livetv.title}
        description={t.livetv.description}
        actions={
          <ButtonLink to={settingsPath('recordings')} icon={RecordIcon}>
            {t.livetv.recordings}
          </ButtonLink>
        }
      />
      <LibraryEditor scope="shared" />
    </>
  )
}

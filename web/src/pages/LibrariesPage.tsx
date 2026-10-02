import LibraryEditor from '@/components/LibraryEditor'
import { PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'

export default function LibrariesPage() {
  const { t } = useI18n()
  return (
    <>
      <PageHeader title={t.libraries.title} description={t.libraries.description} />
      <LibraryEditor scope="shared" />
    </>
  )
}

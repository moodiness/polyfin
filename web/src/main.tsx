import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { queryClient } from '@/api'
import { router } from '@/app/router'
import { answerAppNavigation } from '@/appNavigation'
import { LanguageProvider, loadInitialLanguage } from '@/i18n'
import { showKeptInterfaceSize } from '@/interfaceSize'
import { ToastProvider } from '@/ui'
import '@/index.css'

// Before anything is drawn, so that the first frame has the size the session then confirms.
showKeptInterfaceSize()
answerAppNavigation((delta) => router.navigate(delta))
// A page, a language or the palette's entries load as their own files when first needed. After an
// update of Polyfin, a tab still open asks for its version's files, which are gone: it then loads
// the new version, once a minute at most, so that a broken build cannot reload for ever.
window.addEventListener('vite:preloadError', () => {
  const key = 'polyfin.reloadedForUpdate'
  const last = Number(window.sessionStorage.getItem(key) ?? 0)
  if (Date.now() - last < 60_000) return
  window.sessionStorage.setItem(key, String(Date.now()))
  window.location.reload()
})

const root = document.getElementById('root')
if (!root) throw new Error('Missing #root element')

// The interface's language is a file of its own: drawn once its messages are in, while the router
// already downloads the page's own file.
const initial = await loadInitialLanguage()
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <LanguageProvider initial={initial}>
        <ToastProvider>
          <RouterProvider router={router} />
        </ToastProvider>
      </LanguageProvider>
    </QueryClientProvider>
  </StrictMode>,
)

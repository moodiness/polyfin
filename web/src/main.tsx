import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { queryClient } from '@/api'
import { router } from '@/app/router'
import { answerAppNavigation } from '@/appNavigation'
import { LanguageProvider } from '@/i18n'
import { ToastProvider } from '@/ui'
import '@/index.css'

answerAppNavigation((delta) => router.navigate(delta))

const root = document.getElementById('root')
if (!root) throw new Error('Missing #root element')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <LanguageProvider>
        <ToastProvider>
          <RouterProvider router={router} />
        </ToastProvider>
      </LanguageProvider>
    </QueryClientProvider>
  </StrictMode>,
)

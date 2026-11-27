import { createContext, use, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

/** The end of the Add a source panel's footer, where the form of the kind shown puts its buttons. */
export const AddFooter = createContext<HTMLElement | null>(null)

/**
 * A form's buttons in the Add a source panel's footer, right of its Cancel, where they stay put
 * while the form scrolls. A submit button names its form (`form={id}`), being outside it.
 */
export function AddActions({ children }: { children: ReactNode }) {
  const footer = use(AddFooter)
  return footer === null ? null : createPortal(children, footer)
}

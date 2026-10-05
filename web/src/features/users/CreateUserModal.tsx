import { useMutation } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { ApiError, createUser, queryClient, queryKeys, type User } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Checkbox, Modal, Field, Notice, TextInput, useToast } from '@/ui'

/** Codes about one field, shown under it rather than above the buttons. */
const nameCodes = ['invalid_name', 'name_taken']
const passwordCodes = ['invalid_password']

/** The form to create a user, in a floating panel over the list. */
export default function CreateUserModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const formId = useId()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [isAdministrator, setIsAdministrator] = useState(false)
  // Hidden by default, like Jellyfin.
  const [showOnSignIn, setShowOnSignIn] = useState(false)

  const mutation = useMutation({
    mutationFn: createUser,
    onSuccess: (created) => {
      setName('')
      setPassword('')
      setIsAdministrator(false)
      setShowOnSignIn(false)
      // The new user's page needs it in the list before the refresh comes back.
      queryClient.setQueryData<User[]>(queryKeys.users, (old) => old && [...old, created])
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(t.users.created(created.name))
      onClose()
      void navigate(`/users/${created.id}`)
    },
  })

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const nameError = code !== null && nameCodes.includes(code) ? message : undefined
  const passwordError = code !== null && passwordCodes.includes(code) ? message : undefined
  const otherError = mutation.isError && !nameError && !passwordError ? message : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ name: name.trim(), password, isAdministrator, isHidden: !showOnSignIn })
  }

  return (
    <Modal
      open={open}
      onClose={() => {
        mutation.reset()
        onClose()
      }}
      title={t.users.createTitle}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={mutation.isPending}>
            {mutation.isPending ? t.users.creating : t.users.create}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <Field label={t.users.name} help={t.common.nameRule} error={nameError}>
          <TextInput
            value={name}
            onValue={(value) => {
              mutation.reset()
              setName(value)
            }}
            autoComplete="off"
            maxLength={64}
            required
            invalid={nameError !== undefined}
          />
        </Field>
        <Field label={t.users.password} help={t.common.passwordRule} error={passwordError}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => {
              mutation.reset()
              setPassword(value)
            }}
            autoComplete="new-password"
            required
            invalid={passwordError !== undefined}
          />
        </Field>
        <Checkbox
          label={t.users.isAdministrator}
          help={t.users.isAdministratorHelp}
          checked={isAdministrator}
          onChange={setIsAdministrator}
        />
        <Checkbox
          label={t.users.showOnSignIn}
          help={t.users.showOnSignInHelp}
          checked={showOnSignIn}
          onChange={setShowOnSignIn}
        />
        {otherError && <Notice tone="danger">{otherError}</Notice>}
      </form>
    </Modal>
  )
}

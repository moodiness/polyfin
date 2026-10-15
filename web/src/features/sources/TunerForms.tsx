import { CellTowerIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { addIptvSource, ApiError, findTuners, queryClient, queryKeys, type Addon } from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  ButtonLink,
  Checkbox,
  Field,
  FieldError,
  Notice,
  Row,
  RowList,
  TextInput,
} from '@/ui'
import { invalidateScope, sourcePath } from './model'

/** A failed add, told for a tuner: an address the server refuses says what a tuner's looks like. */
function addError(t: Messages, error: unknown): string {
  return error instanceof ApiError && error.code === 'invalid_source_address'
    ? t.sourceAdd.tunerAddressInvalid
    : errorMessage(t, error)
}

/**
 * Adds an HDHomeRun tuner to the server's sources: one the local network answers for, or one
 * given by its address. Tuners are the server's only, added by administrators.
 */
export function TunerAddFlow({ onAdded }: { onAdded: (added: Addon) => void }) {
  const { language, t } = useI18n()
  const text = t.sourceAdd
  const [favoritesOnly, setFavoritesOnly] = useState(false)
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const find = useMutation({ mutationFn: () => findTuners() })
  const add = useMutation({
    mutationFn: (tuner: { url: string; name: string; found: boolean }) =>
      addIptvSource('shared', {
        kind: 'hdhomerun',
        url: tuner.url,
        name: tuner.name,
        options: { favoritesOnly },
      }),
    onSuccess: (added) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons('shared'), (old) =>
        old === undefined ? old : [...old, added],
      )
      onAdded(added)
    },
    onSettled: () => invalidateScope('shared'),
  })
  const adding = add.isPending ? add.variables : undefined
  const failed = add.isError ? add.variables : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    add.mutate({ url: address.trim(), name: name.trim(), found: false })
  }

  return (
    <div className="flex flex-col gap-6">
      <p className="text-small text-ink-3">{text.tunerHelp}</p>
      <Checkbox
        label={text.favoritesOnly}
        help={text.favoritesOnlyHelp}
        checked={favoritesOnly}
        onChange={(checked) => {
          add.reset()
          setFavoritesOnly(checked)
        }}
      />
      <section className="flex flex-col gap-4 border-t border-line pt-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <h3 className="text-control font-medium text-ink">{text.foundTitle}</h3>
            <p className="mt-1 text-small text-ink-3">
              {find.isPending ? text.finding : text.findTunersHelp}
            </p>
          </div>
          <Button
            icon={MagnifyingGlassIcon}
            loading={find.isPending}
            onClick={() => {
              add.reset()
              find.mutate()
            }}
          >
            {text.findTuners}
          </Button>
        </div>
        {find.isError && <FieldError>{errorMessage(t, find.error)}</FieldError>}
        {find.isSuccess &&
          (find.data.items.length === 0 ? (
            <Notice live>
              <span className="font-medium">{text.noTuners}</span> {text.noTunersHelp}
            </Notice>
          ) : (
            <RowList aria-label={text.foundTitle}>
              {find.data.items.map((tuner) => (
                <Row
                  key={tuner.deviceId}
                  leading={CellTowerIcon}
                  title={tuner.name || tuner.model}
                  titleAside={tuner.source !== null && <Badge>{text.tunerAdded}</Badge>}
                  meta={
                    <span className="truncate">
                      {[
                        tuner.model,
                        text.tunerCount(tuner.tuners, tuner.tuners.toLocaleString(language)),
                        tuner.address,
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </span>
                  }
                  trailing={
                    tuner.source !== null ? (
                      <ButtonLink
                        size="sm"
                        to={sourcePath('shared', tuner.source)}
                        aria-label={text.openTuner(tuner.name || tuner.model)}
                      >
                        {t.sources.openSource}
                      </ButtonLink>
                    ) : (
                      <Button
                        size="sm"
                        variant="primary"
                        loading={adding?.found === true && adding.url === tuner.address}
                        disabled={add.isPending}
                        aria-label={text.addTunerLabel(tuner.name || tuner.model)}
                        onClick={() => add.mutate({ url: tuner.address, name: '', found: true })}
                      >
                        {text.addTuner}
                      </Button>
                    )
                  }
                />
              ))}
            </RowList>
          ))}
        {failed?.found === true && <FieldError>{addError(t, add.error)}</FieldError>}
      </section>
      <form onSubmit={submit} noValidate className="flex flex-col gap-5 border-t border-line pt-5">
        <Field
          label={text.tunerAddress}
          help={text.tunerAddressHint}
          error={failed?.found === false ? addError(t, add.error) : undefined}
        >
          <TextInput
            inputMode="url"
            value={address}
            onValue={(value) => {
              add.reset()
              setAddress(value)
            }}
            placeholder="192.168.1.20"
            autoComplete="off"
            spellCheck={false}
            required
            mono
          />
        </Field>
        <Field label={text.tunerName} help={text.tunerNameHint}>
          <TextInput
            value={name}
            onValue={(value) => {
              add.reset()
              setName(value)
            }}
            maxLength={64}
            autoComplete="off"
          />
        </Field>
        <div>
          <Button
            type="submit"
            variant="primary"
            loading={adding?.found === false}
            disabled={address.trim() === '' || add.isPending}
          >
            {text.byAddress}
          </Button>
        </div>
      </form>
    </div>
  )
}

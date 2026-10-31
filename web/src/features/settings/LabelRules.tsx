import { PlusIcon, XIcon } from '@phosphor-icons/react'
import { useId } from 'react'
import type { VersionLabelRule } from '@/api'
import { useI18n } from '@/i18n'
import { Button, Checkbox, FieldError, IconButton, NumberInput, Select, TextInput } from '@/ui'

/** What a rule does with the versions it matches. */
type RuleAction = 'score' | 'hide'

/** The longest rule and the highest score, either way, the server takes. */
const maxLength = 200
const maxScore = 100

/**
 * The label rules, in order: each row is a word or a regular expression, and what it does with
 * the versions whose labels it matches, adding its score to theirs or hiding them; a button adds
 * a rule, up to `max`.
 */
export default function LabelRules({
  rules,
  max,
  onChange,
  error,
}: {
  rules: VersionLabelRule[]
  max: number
  onChange: (rules: VersionLabelRule[]) => void
  error?: string
}) {
  const { t } = useI18n()
  const text = t.settings.versionLabelRules
  const id = useId()

  function change(index: number, changes: Partial<VersionLabelRule>) {
    onChange(rules.map((rule, at) => (at === index ? { ...rule, ...changes } : rule)))
  }

  return (
    <div role="group" aria-labelledby={`${id}-label`} aria-describedby={`${id}-help`}>
      <p id={`${id}-label`} className="text-[15px] font-medium tracking-[-0.01em] text-ink">
        {text.label}
      </p>
      <p id={`${id}-help`} className="mt-1 max-w-[60ch] text-small text-ink-3">
        {text.help(String(max))}
      </p>
      {rules.length === 0 ? (
        <p className="mt-3.5 text-small text-ink-2">{text.none}</p>
      ) : (
        <ol className="mt-3.5 rounded-row border border-line-2 bg-s1">
          {rules.map((rule, index) => (
            <li
              key={index}
              className="flex flex-wrap items-center gap-3 py-2 pr-2.5 pl-3.5 not-first:border-t not-first:border-line"
            >
              <TextInput
                aria-label={text.text(index + 1)}
                placeholder={rule.regex ? text.regexPlaceholder : text.wordPlaceholder}
                value={rule.text}
                maxLength={maxLength}
                onValue={(value) => change(index, { text: value })}
                mono={rule.regex}
                spellCheck={false}
                autoComplete="off"
                className="min-w-0 flex-1 basis-56"
              />
              <Checkbox
                label={text.regex}
                checked={rule.regex}
                onChange={(regex) => change(index, { regex })}
              />
              <Select
                aria-label={text.action(index + 1)}
                value={(rule.hide ? 'hide' : 'score') as RuleAction}
                options={[
                  { value: 'score' as RuleAction, label: text.score },
                  { value: 'hide' as RuleAction, label: text.hide },
                ]}
                onValue={(action: RuleAction) =>
                  change(
                    index,
                    action === 'hide'
                      ? { hide: true, score: 0 }
                      : { hide: false, score: rule.score || 10 },
                  )
                }
                className="w-40"
              />
              {!rule.hide && (
                <NumberInput
                  aria-label={text.points(index + 1)}
                  min={-maxScore}
                  max={maxScore}
                  step={1}
                  value={rule.score}
                  onValue={(value) => change(index, { score: Math.trunc(value ?? 0) })}
                />
              )}
              <IconButton
                size="sm"
                icon={XIcon}
                danger
                label={text.remove(index + 1)}
                onClick={() => onChange(rules.filter((_, at) => at !== index))}
              />
            </li>
          ))}
        </ol>
      )}
      <Button
        className="mt-3"
        size="sm"
        icon={PlusIcon}
        disabled={rules.length >= max}
        onClick={() => onChange([...rules, { text: '', regex: false, score: 10, hide: false }])}
      >
        {text.add}
      </Button>
      {error && (
        <div className="mt-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}

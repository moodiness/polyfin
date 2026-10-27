import { useState } from 'react'
import { useI18n } from '@/i18n'
import { NumberInput, SecretField, Select, Textarea, TextInput } from '@/ui'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow } from './parts'
import type { SectionFormApi } from './SectionForm'
import WordList from './WordList'

type Route = 'interface' | 'proxy' | 'wireguard'

/**
 * Settings › Torrents: playing the torrents addons list, over one route alone, a network interface
 * (Linux only), a SOCKS5 proxy or a WireGuard tunnel Polyfin runs, and the limits of their traffic.
 */
export default function TorrentsSection({
  form,
  update,
  error,
  number,
  range,
  limits,
}: SectionFormApi) {
  const { t } = useI18n()
  const text = t.settings.torrents
  // Where the interface is not offered, the proxy and the tunnel are the routes.
  const [route, setRoute] = useState<Route>(
    form.interfaceRoutes && form.torrentInterface !== ''
      ? 'interface'
      : form.torrentWireGuardSaved !== null && form.torrentProxy === ''
        ? 'wireguard'
        : 'proxy',
  )
  const choose = (next: Route) => {
    setRoute(next)
    // One route alone: the others are cleared, a saved WireGuard configuration removed; back to
    // the tunnel, the saved one is kept.
    if (next === 'interface') update({ torrentProxy: '', torrentWireGuard: '' })
    else if (next === 'proxy') update({ torrentInterface: '', torrentWireGuard: '' })
    else update({ torrentInterface: '', torrentProxy: '', torrentWireGuard: undefined })
  }
  const routes: Route[] = form.interfaceRoutes
    ? ['interface', 'proxy', 'wireguard']
    : ['proxy', 'wireguard']
  const saved = form.torrentWireGuardSaved
  return (
    <SettingsGroup>
      <SwitchRow
        anchor="torrent-streams"
        label={text.play}
        help={text.playHelp}
        checked={form.torrentStreams}
        onChange={(torrentStreams) => update({ torrentStreams })}
      />
      <FieldRow
        anchor="torrent-route"
        label={text.route}
        help={form.interfaceRoutes ? text.routeHelp : text.routeHelpNoInterface}
        error={error('torrent-route')}
      >
        <Select
          value={route}
          options={routes.map((value) => ({ value, label: text.routes[value] }))}
          onValue={choose}
          className="max-w-xs"
        />
      </FieldRow>
      {route === 'wireguard' && (
        <FieldRow
          anchor="torrent-wireguard"
          label={text.wireGuard}
          help={
            <>
              {text.wireGuardHelp}
              {saved !== null && form.torrentWireGuard !== '' && (
                <>
                  {' '}
                  {text.wireGuardSaved(saved.endpoint, saved.addresses.join(', '))}{' '}
                  {saved.dns.length > 0
                    ? text.wireGuardDns(saved.dns.join(', '))
                    : text.wireGuardNoDns}
                </>
              )}
            </>
          }
          error={error('torrent-wireguard')}
        >
          <Textarea
            value={form.torrentWireGuard ?? ''}
            onValue={(torrentWireGuard) =>
              // Emptied, the field keeps the saved configuration rather than removing it.
              update({ torrentWireGuard: torrentWireGuard === '' ? undefined : torrentWireGuard })
            }
            placeholder={
              saved !== null && form.torrentWireGuard !== '' ? text.wireGuardReplace : '[Interface]'
            }
            maxLength={limits('torrentWireGuard').max}
            rows={8}
            mono
            spellCheck={false}
            autoComplete="off"
            invalid={error('torrent-wireguard') !== undefined}
          />
        </FieldRow>
      )}
      {route === 'interface' && (
        <FieldRow
          anchor="torrent-interface"
          label={text.interface}
          help={text.interfaceHelp}
          error={error('torrent-interface')}
        >
          <TextInput
            value={form.torrentInterface}
            onValue={(torrentInterface) => update({ torrentInterface })}
            placeholder="wg0"
            maxLength={limits('torrentInterface').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-xs"
          />
        </FieldRow>
      )}
      {route === 'proxy' && (
        <>
          <FieldRow
            anchor="torrent-proxy"
            label={text.proxy}
            help={text.proxyHelp}
            error={error('torrent-proxy')}
          >
            <TextInput
              value={form.torrentProxy}
              onValue={(torrentProxy) => update({ torrentProxy })}
              placeholder="10.0.0.2:1080"
              maxLength={limits('torrentProxy').max}
              autoComplete="off"
              spellCheck={false}
              mono
              className="max-w-sm"
            />
          </FieldRow>
          <FieldRow
            anchor="torrent-proxy-user"
            label={text.proxyUser}
            help={text.proxyUserHelp}
            error={error('torrent-proxy-user')}
          >
            <TextInput
              value={form.torrentProxyUser}
              onValue={(torrentProxyUser) => update({ torrentProxyUser })}
              maxLength={limits('torrentProxyUser').max}
              autoComplete="off"
              spellCheck={false}
              className="max-w-sm"
            />
          </FieldRow>
          <SettingRow
            anchor="torrent-proxy-password"
            className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
          >
            <SecretField
              label={text.proxyPassword}
              help={text.proxyPasswordHelp}
              saved={form.torrentProxyPasswordSet}
              value={form.torrentProxyPassword}
              onValue={(torrentProxyPassword) => update({ torrentProxyPassword })}
            />
          </SettingRow>
        </>
      )}
      <FieldRow
        anchor="torrent-download-limit"
        label={text.downloadLimit}
        help={text.downloadLimitHelp(range('torrentDownloadMbps'))}
        error={error('torrent-download-limit')}
      >
        <NumberInput
          {...limits('torrentDownloadMbps')}
          step={1}
          {...number('torrent-download-limit', form.torrentDownloadMbps, (value) =>
            update({ torrentDownloadMbps: Math.trunc(value) }),
          )}
          className="max-w-[10rem]"
        />
      </FieldRow>
      <FieldRow
        anchor="torrent-upload-limit"
        label={text.uploadLimit}
        help={text.uploadLimitHelp(range('torrentUploadMbps'))}
        error={error('torrent-upload-limit')}
      >
        <NumberInput
          {...limits('torrentUploadMbps')}
          step={1}
          {...number('torrent-upload-limit', form.torrentUploadMbps, (value) =>
            update({ torrentUploadMbps: Math.trunc(value) }),
          )}
          className="max-w-[10rem]"
        />
      </FieldRow>
      <FieldRow
        anchor="torrents-at-once"
        label={text.atOnce}
        help={text.atOnceHelp(range('torrentsAtOnce'))}
        error={error('torrents-at-once')}
      >
        <NumberInput
          {...limits('torrentsAtOnce')}
          step={1}
          {...number('torrents-at-once', form.torrentsAtOnce, (value) =>
            update({ torrentsAtOnce: Math.trunc(value) }),
          )}
          className="max-w-[10rem]"
        />
      </FieldRow>
      <FieldRow
        anchor="torrent-trackers"
        label={text.trackers}
        help={text.trackersHelp(range('torrentTrackers'))}
        error={error('torrent-trackers')}
      >
        <WordList
          value={form.torrentTrackers}
          onValue={(torrentTrackers) => update({ torrentTrackers })}
        />
      </FieldRow>
    </SettingsGroup>
  )
}

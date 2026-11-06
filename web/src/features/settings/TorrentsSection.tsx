import { useState } from 'react'
import { useI18n } from '@/i18n'
import { SettingRow, Textarea } from '@/ui'
import { FieldRow, SettingsGroup } from './parts'
import type { SectionFormApi } from './SectionForm'
import WordList from './WordList'

type Route = 'interface' | 'proxy' | 'wireguard'

/**
 * Settings › Torrents: playing the torrents addons list, over one route alone, a network interface
 * (Linux only), a SOCKS5 proxy or a WireGuard tunnel Polyfin runs, and the limits of their traffic.
 */
export default function TorrentsSection({
  form,
  saved,
  update,
  error,
  number,
  range,
  limits,
  minMax,
}: SectionFormApi) {
  const { t } = useI18n()
  const text = t.settings.torrents
  // Where the interface is not offered, the proxy and the tunnel are the routes.
  const routeOf = (settings: typeof form): Route =>
    settings.interfaceRoutes && settings.torrentInterface !== ''
      ? 'interface'
      : settings.torrentWireGuardSaved !== null && settings.torrentProxy === ''
        ? 'wireguard'
        : 'proxy'
  const [route, setRoute] = useState<Route>(() => routeOf(form))
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
  const wg = form.torrentWireGuardSaved
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="torrent-streams"
        label={text.play}
        help={text.playHelp}
        checked={form.torrentStreams}
        saved={saved.torrentStreams}
        onChange={(torrentStreams) => update({ torrentStreams })}
      />
      <SettingRow
        kind="choice"
        anchor="torrent-route"
        label={text.route}
        help={form.interfaceRoutes ? text.routeHelp : text.routeHelpNoInterface}
        error={error('torrent-route')}
        value={route}
        saved={routeOf(saved)}
        options={routes.map((value) => ({ value, label: text.routes[value] }))}
        onChange={choose}
      />
      {route === 'wireguard' && (
        <FieldRow
          anchor="torrent-wireguard"
          label={text.wireGuard}
          help={
            <>
              {text.wireGuardHelp}
              {wg !== null && form.torrentWireGuard !== '' && (
                <>
                  {' '}
                  {text.wireGuardSaved(wg.endpoint, wg.addresses.join(', '))}{' '}
                  {wg.dns.length > 0 ? text.wireGuardDns(wg.dns.join(', ')) : text.wireGuardNoDns}
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
              wg !== null && form.torrentWireGuard !== '' ? text.wireGuardReplace : '[Interface]'
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
        <SettingRow
          kind="text"
          anchor="torrent-interface"
          label={text.interface}
          help={text.interfaceHelp}
          error={error('torrent-interface')}
          value={form.torrentInterface}
          saved={saved.torrentInterface}
          onChange={(torrentInterface) => update({ torrentInterface })}
          placeholder="wg0"
          maxLength={limits('torrentInterface').max}
          mono
        />
      )}
      {route === 'proxy' && (
        <>
          <SettingRow
            kind="text"
            anchor="torrent-proxy"
            label={text.proxy}
            help={text.proxyHelp}
            error={error('torrent-proxy')}
            value={form.torrentProxy}
            saved={saved.torrentProxy}
            onChange={(torrentProxy) => update({ torrentProxy })}
            placeholder="10.0.0.2:1080"
            maxLength={limits('torrentProxy').max}
            mono
          />
          <SettingRow
            kind="text"
            anchor="torrent-proxy-user"
            label={text.proxyUser}
            help={text.proxyUserHelp}
            error={error('torrent-proxy-user')}
            value={form.torrentProxyUser}
            saved={saved.torrentProxyUser}
            onChange={(torrentProxyUser) => update({ torrentProxyUser })}
            maxLength={limits('torrentProxyUser').max}
          />
          <SettingRow
            kind="secret"
            anchor="torrent-proxy-password"
            label={text.proxyPassword}
            help={text.proxyPasswordHelp}
            error={error('torrent-proxy-password')}
            saved={form.torrentProxyPasswordSet}
            value={form.torrentProxyPassword}
            onValue={(torrentProxyPassword) => update({ torrentProxyPassword })}
          />
        </>
      )}
      <SettingRow
        kind="number"
        anchor="torrent-download-limit"
        label={text.downloadLimit}
        help={text.downloadLimitHelp(range('torrentDownloadMbps'))}
        error={error('torrent-download-limit')}
        {...minMax('torrentDownloadMbps')}
        step={1}
        saved={saved.torrentDownloadMbps}
        {...number('torrent-download-limit', form.torrentDownloadMbps, (value) =>
          update({ torrentDownloadMbps: Math.trunc(value) }),
        )}
      />
      <SettingRow
        kind="number"
        anchor="torrent-upload-limit"
        label={text.uploadLimit}
        help={text.uploadLimitHelp(range('torrentUploadMbps'))}
        error={error('torrent-upload-limit')}
        {...minMax('torrentUploadMbps')}
        step={1}
        saved={saved.torrentUploadMbps}
        {...number('torrent-upload-limit', form.torrentUploadMbps, (value) =>
          update({ torrentUploadMbps: Math.trunc(value) }),
        )}
      />
      <SettingRow
        kind="number"
        anchor="torrents-at-once"
        label={text.atOnce}
        help={text.atOnceHelp(range('torrentsAtOnce'))}
        error={error('torrents-at-once')}
        {...minMax('torrentsAtOnce')}
        step={1}
        saved={saved.torrentsAtOnce}
        {...number('torrents-at-once', form.torrentsAtOnce, (value) =>
          update({ torrentsAtOnce: Math.trunc(value) }),
        )}
      />
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

import common from './common'
import nav from './nav'
import home from './home'
import auth from './auth'
import sources from './sources'
import iptv from './iptv'
import libraries from './libraries'
import livetv from './livetv'
import users from './users'
import system from './system'
import settings from './settings'
import account from './account'
import notifications from './notifications'
import statistics from './statistics'
import backups from './backups'
import versions from './versions'

const messages = {
  ...common,
  ...nav,
  ...home,
  ...auth,
  ...sources,
  ...iptv,
  ...libraries,
  ...livetv,
  ...users,
  ...system,
  ...settings,
  ...account,
  ...notifications,
  ...statistics,
  ...backups,
  ...versions,
}

// Two areas defining the same top-level key would silently hide one of them.
if (import.meta.env.DEV) {
  const seen = new Set<string>()
  for (const area of [
    common,
    nav,
    home,
    auth,
    sources,
    iptv,
    libraries,
    livetv,
    users,
    system,
    settings,
    account,
    notifications,
    statistics,
    backups,
    versions,
  ]) {
    for (const key of Object.keys(area)) {
      if (seen.has(key)) throw new Error(`i18n: the key "${key}" is defined by two areas`)
      seen.add(key)
    }
  }
}

export default messages

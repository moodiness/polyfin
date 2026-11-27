import type { UpFrontMessages } from '../index'
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
import account from './account'
import notifications from './notifications'
import statistics from './statistics'
import backups from './backups'
import versions from './versions'

const messages: UpFrontMessages = {
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
  ...account,
  ...notifications,
  ...statistics,
  ...backups,
  ...versions,
}

export default messages

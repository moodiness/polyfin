# Security policy

## Supported code

Security fixes go to the `main` branch and ship in the next release; only the latest release is supported, and older releases, commits and forks are not maintained. Include the version, commit or image digest you tested when reporting a problem.

## Report a vulnerability privately

**Do not publish vulnerability details in issues, pull requests or discussions.**

Use [GitHub's private vulnerability reporting](https://github.com/moodiness/polyfin/security/advisories/new), enabled for this repository. If that channel is unavailable, [request a private reporting contact](https://github.com/moodiness/polyfin/issues/new?template=security-contact.yml). That request must contain only a request for contact: no exploit, affected endpoint, credentials or technical details. Wait for a private channel before sending the report.

A useful private report includes:

- Affected version, commit or image digest, deployment method and relevant configuration with secrets removed.
- The Jellyfin client and version involved, if any.
- The crossed security boundary and realistic impact.
- Minimal reproduction steps against systems and accounts you control.
- Any known mitigation or proposed fix.

Never send passwords, Jellyfin access tokens or API keys, debrid or provider credentials, addon manifest URLs (they often embed credentials), signed stream URLs or an unredacted database. Revoke any secret exposed during research.

Report vulnerabilities in Stremio addons, debrid services, Jellyfin clients or other dependencies to their own project, unless Polyfin uses them unsafely.

## Disclosure

Keep details private until a fix or advisory is available or a disclosure date is agreed. Test only systems you own or are authorized to assess, minimize access and disruption, avoid social engineering and denial of service, and follow applicable law. No response-time or remediation SLA is promised.

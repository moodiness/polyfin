# Contributing to Polyfin

Thank you for helping. Each kind of contribution has its place:

- **A question or help with a setup:** ask in [Q&A](https://github.com/moodiness/polyfin/discussions/categories/q-a), after searching it and the [documentation](https://moodiness.github.io/polyfin/docs/).
- **An idea:** post it in [Ideas](https://github.com/moodiness/polyfin/discussions/categories/ideas), or vote for an existing one. The ideas that are picked become issues on the [roadmap](https://github.com/users/moodiness/projects/1).
- **A bug:** use the [bug report form](https://github.com/moodiness/polyfin/issues/new?template=bug_report.yml).
- **A vulnerability:** follow the [security policy](SECURITY.md), never a public issue.
- **A fix to the documentation:** use **Edit this page on GitHub** at the bottom of any page of the [website](https://moodiness.github.io/polyfin/docs/).

## Changing the code or the documentation

1. For anything beyond a small fix, open or comment on an issue first, so that the change is agreed before you write it.
2. Run Polyfin from source and its tests as [Development](docs/development.md) describes. `make check` runs the formatting checks, vet and the tests.
3. Keep the documentation in `docs/` in step with the behavior you change.
4. Open a pull request against `main`. Its title follows [Conventional Commits](https://www.conventionalcommits.org/), such as `feat: ...` or `fix(addons): ...`: pull requests are squash-merged, and the title becomes the commit message.

Everyone follows the [code of conduct](CODE_OF_CONDUCT.md). Polyfin does not host or distribute content: never share or request content, links to it, addon manifest URLs or credentials.

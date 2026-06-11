# Contributing to bump-semver

Thanks for your interest in `bump-semver`.

## Scope

`bump-semver` does one thing: **extract and rewrite a semantic version** inside a
file (or read one from a VCS tag / shell command). It is deliberately *not* a
general configuration-management tool — it does not edit arbitrary fields, manage
dependencies, or template files. Requests that fall outside "find/replace the
version string" are usually out of scope.

## Requesting a new built-in format

If a file you use isn't recognized by the built-in table, first check whether you
can already handle it yourself with `--define-rule` (see the
[Custom rules](./README.md#custom-rules---define-rule) section). A custom rule
works immediately, without waiting for a release.

If you'd still like the format to become built-in for everyone, open a
[Built-in format request](https://github.com/kawaz/bump-semver/issues/new?template=format-request.yml).
The form asks for the file name, a short sample, and where the version lives.

## Pull requests

- Tests are required. New formats need a test that proves extraction (and write,
  when applicable) works.
- When a change touches a design decision, reference the relevant record under
  [docs/decisions/](./docs/decisions/) (and add a new DR if you're making a new
  decision).
- Keep the `README.md` and `README-ja.md` in sync — they are a translation pair.

## Review cadence

This is a personal project; the maintainer typically only gets to it on weekends,
so please be patient with response times.

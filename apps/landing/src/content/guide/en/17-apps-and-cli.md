---
title: Apps and command-line tool
order: 17
---

## Desktop app

The Delegora desktop app is available for macOS, Windows and Linux. It uses the same account and data as the web app and manages the daemon on your machine for you. You can be signed in to both at once.

Tabs are saved per workspace. Logging out clears every tab saved on the machine. Desktop-only shortcuts (closing and switching tabs, zoom) work only in the desktop app.

Auto-update is on by default. On Linux, auto-update works only for the `.AppImage` package; for `.deb` and `.rpm`, install the new package over the old one.

## iOS app

The iPhone app is not on the App Store yet. You can build a Release version from the source on a Mac and install it on your own iPhone. A free Apple account signs the app for only 7 days; after that, reconnect the iPhone and rerun the build to re-sign. Your data on the server is unaffected by reinstalling.

## Command-line tool

The command-line tool connects local runtimes and manages workspaces, issues, agents, autopilots, skills and runtimes from a terminal. Install it with the official script or Homebrew, then check with `delegora version`.

Sign in the first time with `delegora setup`: it opens a browser to sign in and starts the daemon. Check with `delegora auth status` and `delegora daemon status`. On a machine without a browser, create a personal access token on the web and run `delegora login --token`.

Common commands:

```bash
delegora workspace list
delegora issue list
delegora issue create --title "Fix login failure"
delegora issue status MUL-123 in_progress
delegora issue comment add MUL-123 --content "Check the regression tests first"
delegora issue runs MUL-123
```

Read long text from a file or stdin to avoid quoting problems:

```bash
delegora issue create --title "Upgrade notes" --description-stdin < notes.md
```

Pass secrets through a file or stdin rather than on the command line, so they stay out of shell history. The configuration file holds a token that can act as you, so never commit it to a repository.

Run `delegora <command> --help` for the flags of your installed version. Use `--output json` in scripts.

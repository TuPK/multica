---
title: Runtimes and machines
order: 5
---

A runtime is one connected machine plus one AI coding tool on it. The daemon is the background process on that machine: it registers the runtime, claims runs from the queue, invokes the local tool and reports results back.

### Supported AI tools

Delegora does not provide the tool's account, subscription or model quota. Install the tool you plan to use, sign in with your own account, and run it once in a terminal. Login credentials stay in the tool; Delegora never receives them. Install only what you need. The daemon needs at least one supported tool to start.

### Adding a machine

- **Desktop app**: registers this computer and detects its tools automatically. It uses its own CLI profile for the daemon and does not overwrite your terminal configuration.
- **Another machine or server**: on **Runtimes**, click **Add**, choose **Other machine or server**, and run the two commands shown.
- **Cloud runtimes**: currently a waiting list.

If a terminal finds a tool but the app does not, the two usually use different `PATH` values. Restart the app, or set an absolute path to the tool.

### Status

- **Online**: a heartbeat arrived in the last 45 seconds. Ready to take work.
- **Recently lost** (under 5 minutes): usually a brief network blip.
- **Offline** (over 5 minutes): check the machine or restart the daemon.
- **Offline for over 6 days**: move its agents to another runtime or delete it.

Queued runs wait for the runtime to return. They are never moved to another machine.

### Concurrency

One daemon runs at most 20 runs at once by default; each agent runs at most 6. The effective limit is the smaller of the two. Runs over the limit wait in the queue.

### Sharing a runtime

A local runtime is private by default: only its owner can create agents on it. The owner can share it with **all workspace members** or with **selected people** from the runtime's detail page. Sharing never lets others rename, delete or administer the runtime. When access is removed, the affected agents are unbound and their runs are cancelled.

### Cleanup and logs

The daemon removes an issue's working directory 24 hours after the issue is closed and has been quiet, and clears regenerable caches such as `node_modules` 12 hours after a run. Chat sessions are cleaned after they are archived. A directory in use is never removed. Read logs with `delegora daemon logs`.

### Custom runtime profiles

If your team uses an internal wrapper, create a **custom runtime profile**: choose a compatible protocol family, then enter a name, a command and fixed arguments. The command is a program and its arguments, not a shell script. For pipes, redirects or `&&`, put them in a script and use the script as the command.

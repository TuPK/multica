---
title: Troubleshooting
order: 16
---

First work out which layer the problem is in: the Delegora service, the daemon, the runtime, or the AI tool. Usually the run status and the runtime status are enough to find it.

## An agent does not run

Open the issue's **execution log** and read the run's status.

- **Queued**: check that the agent's runtime is online, that the tool is detected on it, that the agent still has concurrency headroom, and that the daemon has capacity. If the tool is missing from the runtime list, confirm it runs and is signed in under the same system account and `PATH`, then restart the daemon.
- **Waiting for a local directory**: another run is using the same folder. Switch the resource to parallel (worktree) mode to remove the queue.
- **Runtime offline**: check the machine and the daemon. Queued runs start once the runtime returns.

## Common errors

- **Cannot run on its private runtime**: you do not have access to that runtime. Ask the owner to share it, or bind the agent to a runtime you can use.
- **Model account out of credit**: top up or switch accounts, then retry.
- **Model account not signed in**: sign in again on the machine that runs the agent.
- **Timed out**: split the request into smaller steps.
- **The machine could not set up the run**: usually low disk space, a missing permission, or a locked folder. Read the detailed error below the message.
- **Company paused or budget at its limit**: runs are held. Resume the company or raise the budget.
- **Autopilot run limit reached for the period**: later runs start in the next period.
- **Autopilot paused after losing its runtime**: attach a runtime, then reactivate the autopilot.

## Cannot sign in or connect

- Sign in again in the app, or rerun the sign-in command in the command-line tool.
- If your network uses a proxy or firewall, check that the machine can reach the Delegora service.
- If the web sign-in completes but the command-line tool reports a TLS handshake timeout, retry with the TLS compatibility option described in the technical documentation.

## Email does not arrive

Check your spam folder. Invitations and verification codes expire after a short time; request a new one. If it still does not arrive, contact support with your account email.

## When reporting a problem

Include the command-line tool version, your operating system, the error message and the daemon logs (`delegora daemon logs`). Remove tokens, email addresses and customer data before sending.

---
title: Dynamic workflow, from zero to live
order: 18
---

This chapter is for someone who has never built a workflow. It takes you from an empty squad to a workflow that runs real work, and it explains every control you will meet along the way.

## 1. What you are building

A **squad** is a small team of agents with one leader. Its **workflow** is the path a piece of work takes through that team: who does each step, what a checker looks at, when work goes back for another try, and where it is allowed to finish.

You do not write code to build a workflow. You draw it in the workflow designer, one step at a time, and Delegora checks it before anyone can run it.

These words appear everywhere in the designer:

| Word | Plain meaning |
| --- | --- |
| **Squad** | The team: a leader agent plus member agents. |
| **Workflow** | The whole path. It is a set of steps, roles, edges, loops and stops. |
| **Draft** | A workflow you are still editing. Nothing runs on a draft. |
| **Version** | A published copy. It is locked. Work that is already running keeps the version it started with. |
| **Step** | One place in the path where work sits. |
| **Role** | Who works at a step: an agent, a member, the squad managers, or the person who made the request. |
| **Task** | One concrete piece of work inside a step, for example "write the draft". |
| **Verdict** | The result a task reports: pass, fail or neutral. The next move depends on it. |
| **Gate** | A step whose job is to check someone else's work. |
| **Edge** | An arrow from one step to the next, labelled with when it is taken. |
| **Loop** | An edge that goes back to an earlier step, with a limit on how many times it can happen. |
| **Stop** | A named reason why a request ends without finishing, for example "missing grounds". |
| **Terminal** | A place where a request ends normally. One terminal can take the work out of the workspace. |
| **Request** | One piece of work moving through the workflow. |

## 2. Before you start

You need:

- **Admin or owner access to the workspace**, or a role that can manage the squad. Only squad managers can create and publish a workflow.
- **A squad** with a leader and at least one member agent. Each agent needs a runtime that is online, otherwise its tasks wait.
- **Workflow authoring turned on.** If the designer shows "Workflow authoring is off", the feature is not enabled for this installation. Ask whoever runs the server to enable it.

Who can do what:

| Action | Who |
| --- | --- |
| See the workflow list and read a version | Squad members |
| Create or edit a draft, check, simulate | Squad managers |
| Publish a version | Squad managers |
| Turn on **Auto** for a workflow | Admins only |
| Confirm a workflow that can leave the workspace without a person reviewing it | Admins only |
| Close a task on behalf of an agent | Squad managers (the action is logged) |

## 3. Step 1: Set up the squad

1. Open **Squads** in the sidebar and click **New squad**. Give it a name.
2. Pick the **leader**. The leader plans the work and does not take work tasks itself.
3. Add **member agents** from the squad page. Each agent needs the **skills** its tasks will use. You can add skills later from the agent's page.
4. Make sure each agent's runtime is online. Open the runtime page if an agent shows as offline.

If you already have a ready-made bundle of agents and skills, you can skip the manual setup and import it. That is covered in section 12.

## 4. Step 2: Create a new workflow

Open the squad and scroll to the **Workflows** section. Click **New workflow**. A dialog opens with four ways to start.

**Blank.** You get an empty draft. Type a name and click **Create**. The designer opens and you build every step yourself. Use this when your process is unlike the templates.

**Copy a Kit Việt workflow.** You pick one of the ready Kit Việt templates and get a copy. The copy is separate from the original: changes to the original never reach your copy, and changes to your copy never reach the original. The name is taken from the template unless you type one.

**Import a file.** Choose a `flow.json` file, or paste its contents. The file must be a workflow manifest and must be smaller than 512 KiB. If the file is not valid, the dialog says which part is wrong before anything is created.

**Ask an agent to draft.** Use this when you have a written process and want an agent to turn it into steps:

1. Choose the **agent** that will draft. Pick one that can read documents well.
2. In **What the workflow should do**, describe the process in your own words: who does what, what needs checking, what should happen when a check fails.
3. Under **Reference files**, attach any documents the agent should read, or tick the existing agent instructions and skills of the squad.
4. Click **Ask the agent**. Delegora opens an ordinary issue for the agent, with your notes and the list of squad members. The new draft is empty until the agent finishes.
5. When the agent attaches a `flow.json` file to that issue, the draft page shows **Import proposal**. Click it. The draft is filled with the proposal.
6. Read the proposal in the designer, change what is wrong, then run **Check** and **Simulate** as with any other draft. Publishing needs one more tick, covered in section 9.

The agent only proposes. It cannot publish, cannot assign roles, and cannot switch a workflow on.

## 5. Step 3: Know the designer screen

Open a draft to see the designer. It has three areas.

**The top bar** shows:

- The **workflow name**. Click it to rename.
- A **save status**: "Saving…", "Saved" or "Unsaved changes". The designer saves automatically about 1.5 seconds after your last change. If it cannot save, you see "Could not save" and your change stays on the screen. Copy anything important before you leave.
- **Check**, which lists the problems in the draft.
- **Simulate**, which runs the route without any agents.
- **Export**, which downloads the draft as a `flow.json` file.
- **Publish**, which turns the draft into a locked version. It is greyed out while errors remain.

**The left column, Steps (the outline),** lists every step in the order work flows through it. Use the search box to find a step by name. Click a step to open it. The **Add step** menu at the top of this column adds a new step.

**The middle column, the Inspector,** edits whatever you selected: a step, a task or an edge.

**The right column** holds the side panels. Open each one only when you need it:

- **Roles**: who can do each step, and which agents are in each role.
- **Variables**: named values a workflow can read, such as a number or a yes/no choice.
- **Verdicts**: the sets of results a task can report, and whether each counts as pass or fail.
- **Loops**: how many times each rework loop can run.
- **Stops**: the named reasons a request can stop.
- **Terminals**: the places a request can finish.
- **Reference**: the agent instructions and skills of the squad, shown read-only so you can check the original process while you build.

**The Graph tab** draws the same steps as a map. It is for looking, not for editing:

- Click a step on the map to select it in the Inspector.
- **Fit** resizes the map so every step fits the screen. It never shrinks below half size, so on a large workflow the map scrolls.
- **Find a step** jumps to a step by name.
- **Show loops only** hides everything that is not part of a loop.
- There are no zoom buttons. Scroll the map with the scrollbars or a trackpad.

**There is no drag-and-drop on this map.** You do not move steps or draw arrows by dragging. You make changes in the Inspector and the panels. The only reordering by buttons is for edges: each edge in a step's edge list has up and down arrows. (Drag-and-drop exists only in the older status workflow designer under Settings → Workflows.)

On a phone, the outline shows as a list, and tapping a step opens its Inspector as a full-height sheet. The map becomes a vertical list of steps.

## 6. Step 4: Build your first step

This walkthrough builds a small content workflow. There are four steps:

1. **Draft**: a writer agent writes the text.
2. **Review**: an editor agent checks it. It can pass, send the text back to Draft, or block it.
3. **Approve**: a person from the squad managers approves it.
4. **Done**: the terminal where the work finishes.

Start with the first step.

1. In the **Steps** column, click **Add step** and choose **Work**. A new step appears at the end of the list and opens in the Inspector.
2. In the Inspector, set the **step title**, for example "Draft". Leave the kind as **Tasks**. Leave **Wait for** as **All tasks**: the step is done when all its tasks are done.
3. Under **Tasks**, click **Add task**. Give the task a title, for example "Write the draft". Choose its **kind**: **Work**. The **Role** field is still empty; you fill it in step 5.
4. Write **Instructions** for the task: what the agent does, what the output must contain, and where to put it. Instructions are read by the agent, so be specific.
5. If the task needs a file, tick **Needs an attachment** and say which kind of file. A task with a required file is not finished until the agent attaches that file.

## 7. Step 5: Create roles and fill them

A role is a seat in the workflow. Tasks are given to roles, not directly to agents. This lets you change the agent later without touching the steps.

1. Open the **Roles** panel on the right and click **Add role**.
2. Give it a **key** (a short name such as `writer`) and a **label** (what people see, such as "Writer"). Choose the kind:
   - **Agent**: one or more agents from the squad.
   - **Member**: a human member of the workspace.
   - **Squad managers**: anyone who manages the squad.
   - **Requester**: the person who created the request.
3. For an Agent role, use **Choose an agent** to pick the agents that may take it. The **Pool** column shows how many. If an agent you need is missing, use **Add agent** to add it to the squad first.
4. Back in the Inspector, set the task's **Role** to `writer`.

The leader cannot be put into a task role. A role that no task uses is flagged as unused, and Check reports it.

## 8. Step 6: Add a review step and connect the steps

A gate is the step that checks someone else's work.

1. **Add step** → **Gate**. A gate comes with three ready-made edges and a checking task:
   - **Passed**: goes forward.
   - **Send back for rework**: goes back to the step that did the work, while rework is still allowed.
   - **Blocked**: stops the request as stalled, and a person must look at it.
2. Rename the gate step to "Review". Open its task, rename it "Check the draft", and give it a new role, `editor`. Make sure `editor` is a different agent from `writer`. A gate may not be checked by the agent who made the work it judges. Check reports this as a separation problem.
3. Change the **Send back for rework** edge so it points to "Draft". Each edge has a **Destination**: a step, a terminal, or a stop.
4. Change the **Passed** edge so it points to "Approve", the human step you will add next.

Where the edge list is, and how to use it:

- Each step lists its outgoing edges in order. The order matters: edges are checked from top to bottom, and the last one is the fallback.
- Use the up and down arrows to change the order.
- Use **Add edge** to add a new one, and the trash icon to delete one.
- An edge can have a **condition**, built with the condition builder. The builder compares a fact or variable and combines rules with AND, OR and NOT. Leave the condition empty for an edge that is always taken.

## 9. Step 7: Add a human approval and a loop limit

**Add a human approval.** Click **Add step** → **Human approval**. It is a manual step: a request waits there until a person acts.

- In the Inspector, set **Exit role** to **Squad managers**. Only these people can move the request out of this step.
- Set the **Notice message**. It is posted when a request enters the step, so the approver knows what to do.
- Connect the Review step's **Passed** edge to this step, and connect this step's manual edge to **Done**.

**Limit the rework loop.** The Review step's **Send back for rework** edge is a loop. Open the **Loops** panel:

- Find the loop, for example `review`.
- Set **Up to N times**, for example 2. After two returns to Draft, the next fail follows the edge you set for "loop used up", or the request stops at the **Loop used up** stop.
- Always give a loop an edge for "used up". Check reports a step that goes back but has no later edge for that case.

**Add the terminal.** Open the **Terminals** panel. A terminal named **Done** already exists in most templates. Make sure the Approve step's exit goes to it. If a terminal is marked **Leaves the workspace**, the work leaves the workspace when the request ends there. Such a terminal needs a person to review the work first, and a workflow with one of these may show the release warning.

**Stops.** Open the **Stops** panel to see the stop reasons. Built-in stops are **Missing grounds** and **Loop used up**. You can add custom stops with a message. The message can use values from variables, and you can preview it before saving.

## 10. Step 8: Check the draft

Click **Check**. A list of problems opens, grouped by step. Click a line to jump to the exact place.

Problems you will meet most often, and what to do:

| Message | What it means | What to do |
| --- | --- | --- |
| A step cannot be reached from the start | No edge leads to that step | Add an edge into it, or delete it |
| A role needs N bound, has M | A role has fewer agents than its tasks need | Add agents to the role's pool |
| Gate cannot be given to anyone who did the work it judges | The same agent is both worker and checker | Give the gate to a different agent |
| A role is a person and has no skills | A role of kind Member has no skills | Give the role a skill, or change its kind |
| A step goes back but has no later edge for when the loop is used up | A loop has no exit | Add an edge for "loop used up" |
| A step is on a cycle with no loop edge | Work can circle forever | Mark the going-back edge as a loop |
| No path leads from the start to a terminal without a stop | A request can never finish | Connect a path to a terminal |
| A manual step has no manual edge | A request would wait forever | Add the manual edge |
| A manual step posts no notice | Approvers are not told what to do | Set the notice message |
| A terminal leaves the workspace without a human review | The release warning | Add a human approval before it, or get an admin to confirm |
| A role is unused | No task uses it | Use it, or delete it |

Errors block **Publish**. Warnings do not block it, but read each one. A warning that needs acknowledgement will ask you to tick a box before you publish.

## 11. Step 9: Simulate the route

Click **Simulate**. Choose a value for each variable, or leave a variable on **Default**. Click **Run**.

You see:

- **Task results**, one row for each time a step is entered.
- **Path**, the order of steps and edges that were taken, with a visit number.
- **Ends at**, where the simulation finished.
- **Loops**, how many times each loop ran.

Read the path with two questions: does every step go where you expect, and does every request end somewhere? Simulate only checks the routing. It does not check whether the agents do good work. That is what trial runs are for.

## 12. Step 10: Publish a version

Click **Publish**. The dialog shows:

- A note: publishing creates a locked version, and running requests keep theirs.
- The result of a fresh check. If there are errors, you see "Fix the errors before publishing" and the button stays off.
- **Make this the active workflow**. Tick it to make this version the one new requests use. Leave it unticked to keep the old active version and test the new one with a trial run first.
- If the workflow can leave the workspace without a human review, the release warning appears. Only an admin can tick the confirmation.
- If the draft came from an agent, you must tick **I have read this workflow** before the button enables.

If someone else published a newer version while you were editing, you see "A newer version was published". Click **Edit the latest version** to open it.

After publishing:

- The version appears in the **Workflows** list of the squad with a version number.
- **Auto** is off on the new version, even if the old one was on. An admin turns it back on after reviewing the new version.
- The draft stays in the list under **Drafts** so you can keep working on it. Publishing again makes a new version.

## 13. Step 11: Try it with a trial run

Do not switch a live squad to an untested workflow. Test it first:

1. In the **Workflows** section of the squad, find the version row and click **Trial run**.
2. Enter a title for a sample request, for example "Sample article about coffee". Click **Start**.
3. The request runs with real agents on the same engine. It carries a **Trial** badge.

A trial request is separate from live requests. It does not change how other requests run. It is also left out of the default numbers. Work from a trial cannot leave the workspace until a person has looked at it, so it is safe to try the real path.

## 14. Step 12: Use the workflow for a request

To give a squad real work, create an issue and assign it to the squad.

- If the squad has more than one published workflow, the issue form shows a **workflow** picker. The default is the active version.
- The leader plans the request, and the request then follows the workflow step by step.
- Routine tasks and agent-created requests always use the active version.

## 15. Step 13: Follow a request

Open the issue. The **Flow** panel shows where the request is now:

- **Step**: the current step.
- **Round**: how many rework rounds have been used, shown as `↺ n/max`, for example `↺ 1/2`.
- **Outcome**: Pending, Approved, Rejected, Blocked or Done.
- **Tasks**: each task with its role, status and verdict.
- **Path**: the steps and edges taken so far, with times. Open a line to see the facts behind that choice.
- **Feedback**: comments a rework returned, so the writer knows what to fix.

When a task waits for a decision, choose the verdict that the task allows. The panel only offers verdicts that belong to that task.

Other controls:

- **Close on behalf**: squad managers can close a task in place of the agent. The task shows "Closed by {name} on behalf of the agent". Use it only when the agent cannot finish and you have checked the work.
- **Exit**: take a manual exit, such as Approve and continue, when the step is waiting for a person.
- **Back to drafting** and **Resume**: send a stalled request back to its draft, or resume it. Resume asks you to confirm when it would skip a checkpoint that no person has reviewed.
- **Detach**: separates the request from the workflow engine. Its open tasks become ordinary sub-issues and the workflow stops steering them. This cannot be undone, so use it only when the workflow is broken for this request.

Stop reasons you may see:

| Stop | Meaning |
| --- | --- |
| Stop 1: Blocked | A check found missing grounds: the request lacks what the check needs. |
| Stop 3: Max returns reached | The rework loop was used up. |
| Stalled | The request cannot go on by itself. Someone must act: take the exit, resume, or detach. |

## 16. Step 14: Read the numbers and improve

Open a published version and look at **Insights**. For up to the 500 most recent requests on that version you see:

- how many requests there were, and how many are still open;
- for each step, how many times it was entered, and the median and 90th percentile time spent there;
- for each edge, how many times it was taken;
- for each loop, how many requests returned 0, 1, 2 times and so on.

Tick **Include trial runs** to count trial requests too.

Use the numbers to decide what to change. For example:

- A gate sends work back in most requests: the writer's instructions are probably unclear. Improve the task's instructions.
- A loop is used up often: raise its limit only after you have checked the instructions.
- One step takes days: check whether it has a person waiting, or whether its agent is offline.

Make the change in a new draft, check, simulate, publish, then trial run again. The numbers never change the workflow by themselves.

## 17. Optional: turn on Auto

**Auto** lets the system move a request along its workflow without waking the leader for every small step. Only admins can turn it on, and only on a published version. A new version starts with Auto off.

Turn it on only after you have read the Insights of that version and run a trial. Auto does not change which steps exist; it only changes who moves the request.

## 18. Shortcut: import a whole pack

If you already have a bundle of agents, skills and a flow, you do not have to set everything up by hand.

1. Open **Squads** and click **Import a pack**.
2. Upload a `.zip` file, or paste a GitHub link or a link to a `.zip`.
3. Click **Read the pack**. The preview shows:
   - **Agents**: create each one, use an existing agent, or skip it. For a new agent, pick a runtime.
   - **Skills**: a skill with the same name as one you already have keeps the existing copy, or is imported as a new skill. Nothing is overwritten.
   - **Leader**: pick an agent from the bundle, or use a general leader.
   - **Ignored settings**: tools, models, environment variables and MCP setup inside the bundle are not applied. You set those yourself afterwards.
   - **Security scan**: a blocked bundle cannot be installed. Warnings must be acknowledged with a tick.
4. Click **Install**. You see what was created.
5. If the bundle includes a flow, click **Design the workflow**. A draft opens from that flow. Continue from section 5.

Accepted formats are a bundle with a `squad-bundle.json` file, an agent pack with a `pack.json` file, a folder of agents written as `agents/*.md` (Claude Code style), or folders of skills each with a `SKILL.md` file.

## 19. Problems you may meet

| What you see | Why | What to do |
| --- | --- | --- |
| "Workflow authoring is off" | The feature is not enabled for this installation | Ask whoever runs the server |
| "Could not save" | The network or the server failed | Wait, then check the save status; copy your work if it does not recover |
| "Someone saved this draft first" | Another person changed the draft | Choose **Reload**, or **Keep mine as a copy** to keep your version separately |
| "Fix the errors before publishing" | Check still finds errors | Click the problem line, fix it, check again |
| "A newer version was published" | The version you edited is no longer the latest | Click **Edit the latest version** and redo your change there |
| "Only workspace admins can create statuses" | The workflow uses a status that only admins can create | Ask an admin, or use a status that already exists |
| "The role lacks skills" | A task needs a skill that the agent in its role does not have | Add the skill to the agent, or remove it from the task |
| A request is stalled | No one has acted yet | Open the request and take the next action in the Flow panel |
| Tasks wait and never start | The agent is offline | Check the agent's runtime on its page |

## 20. Words you have seen

- **Active**: the version new requests use.
- **Unreadable**: a saved workflow this version of the app cannot read. Update the app, or ask an admin.
- **Trial**: a request started from a trial run.
- **On behalf**: a close made by a manager in place of the agent.

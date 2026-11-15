import { resolve } from "node:path";
import { expect, type Page } from "@playwright/test";
import { TestApiClient } from "./fixtures";

// Shared by the two flow end-to-end specs (flow-designer-p21-layout, flow-rehearsal-people). Every person of a
// run is a fixture user made through the API, in a workspace of their own, and all of it is removed afterwards.

export const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;

/** A folder for screenshots: the run folder given by the environment, or none. */
export const SHOTS = process.env.FLOW_E2E_SHOTS;

export async function shot(page: Page, name: string) {
  if (SHOTS) await page.screenshot({ path: resolve(SHOTS, `${name}.png`), fullPage: false });
}

export function workspaceSlugOf(api: TestApiClient): string {
  return (api as unknown as { workspaceSlug: string }).workspaceSlug;
}

/** A call to the API as the signed-in user of the client. A non-2xx answer is returned, not thrown. */
export async function rawApi(api: TestApiClient, method: string, path: string, body?: unknown): Promise<{ status: number; json: any }> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${api.getToken()}`, "X-Workspace-Slug": workspaceSlugOf(api) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json: any = undefined;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch {
    json = text;
  }
  return { status: res.status, json };
}

export async function callApi<T = any>(api: TestApiClient, method: string, path: string, body?: unknown): Promise<T> {
  const r = await rawApi(api, method, path, body);
  if (r.status >= 300) throw new Error(`${method} ${path} failed: ${r.status} ${JSON.stringify(r.json)}`);
  return r.json as T;
}

/** A call as an agent, with the token of a task of its own: what the CLI of a running agent does. */
export async function asAgent(api: TestApiClient, agent: { token: string; taskId: string; agentId: string }, method: string, path: string, body?: unknown): Promise<{ status: number; json: any }> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${agent.token}`,
      "X-Workspace-Slug": workspaceSlugOf(api),
      "X-Agent-ID": agent.agentId,
      "X-Task-ID": agent.taskId,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json: any = undefined;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch {
    json = text;
  }
  return { status: res.status, json };
}

export async function noHorizontalScroll(page: Page, where: string) {
  const sizes = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(sizes.scrollWidth, `${where}: the page scrolls sideways (${JSON.stringify(sizes)})`).toBeLessThanOrEqual(sizes.clientWidth);
}

/** Signs a persona in: the token goes where the web app reads it, as the other specs do. */
export async function signIn(page: Page, api: TestApiClient) {
  const token = api.getToken();
  if (!token) throw new Error("the persona has no token");
  await page.addInitScript((t) => {
    localStorage.setItem("multica_token", t);
    localStorage.setItem("multica:chat:isOpen", "false");
  }, token);
}

/** A person of the run: a fixture user who owns, or belongs to, the workspace of the run. */
export async function persona(email: string, name: string): Promise<TestApiClient> {
  const api = new TestApiClient();
  await api.login(email, name);
  await api.markUserOnboarded();
  return api;
}

/** Removes what a run made: its drafts and workflows, its squads and agents, its workspace and its users. */
export async function removeRun(owner: TestApiClient | undefined, emails: string[]) {
  if (owner) {
    try {
      const ws = await owner.queryDb(`SELECT id FROM workspace WHERE slug = $1`, [workspaceSlugOf(owner)]);
      const wsId = ws[0]?.id as string | undefined;
      if (wsId) {
        await owner.queryDb(`UPDATE squad SET active_workflow_id = NULL WHERE workspace_id = $1`, [wsId]);
        await owner.queryDb(`DELETE FROM issue_workflow_draft WHERE workspace_id = $1`, [wsId]).catch(() => undefined);
      }
    } catch {
      /* the workspace may be gone already */
    }
    await owner.cleanup().catch(() => undefined);
    await owner.deleteWorkspace().catch(() => undefined);
  }
  const db = owner ?? new TestApiClient();
  for (const email of emails) {
    await db.queryDb(`DELETE FROM "user" WHERE email = $1`, [email]).catch(() => undefined);
  }
}

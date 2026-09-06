import { test, expect, chromium } from '@playwright/test';
import { startS17Fixture, stopS17Fixture } from './s17-fixture.mjs';

const studioRoot = new URL('../..', import.meta.url).pathname;
const standard = 'S17.SCALE — Draw and interpret scale plans';
const button = (page, name) => page.getByRole('button', { name, exact: true });
const revisionSelect = page => page.getByLabel('Revision', { exact: true });
const nodes = page => page.locator('section.collab-section').filter({ has: page.getByRole('heading', { name: 'Plan nodes', exact: true }) }).locator('ul.collab-list > li > div > strong');
const node = (page, name) => nodes(page).filter({ hasText: new RegExp(`^${name}$`) });

// Each negative action registers one exact method/path/status. Neither console
// nor HTTP errors get a blanket 404/409 allowlist. No headers/cookies are logged.
function audit(page) {
  const expected = [], errors = [], consoleErrors = [], accepted = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('requestfailed', request => errors.push(`${request.method()} ${new URL(request.url()).pathname}: ${request.failure()?.errorText}`));
  page.on('response', response => {
    if (response.status() < 400) return;
    const path = new URL(response.url()).pathname;
    const entry = expected.find(e => !e.seen && e.path === path && e.method === response.request().method() && e.status === response.status());
    if (entry) { entry.seen = true; accepted.push({ url: response.url(), status: response.status(), name: entry.name }); }
    else errors.push(`Unexpected ${response.request().method()} ${path} ${response.status()}`);
  });
  page.on('console', message => {
    if (message.type() === 'error' || message.type() === 'warning') consoleErrors.push({ text: message.text(), url: message.location().url });
  });
  return {
    allow(name, method, path, status) { expected.push({ name, method, path, status, seen: false }); },
    check() {
      for (const message of consoleErrors) {
        const index = accepted.findIndex(e => e.url === message.url && message.text.startsWith('Failed to load resource:') && message.text.includes(String(e.status)));
        if (index >= 0) accepted.splice(index, 1); else errors.push(`Console ${message.url}: ${message.text}`);
      }
      expect(expected.filter(e => !e.seen), 'Every named negative request executed').toEqual([]);
      expect(errors, 'No unexpected network or console failures').toEqual([]);
    },
  };
}

async function request(page, path, method = 'GET', body) {
  return page.evaluate(async ({ path, method, body }) => {
    const response = await fetch(path, { method, credentials: 'include', ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }) });
    const text = await response.text();
    return { status: response.status, body: text ? JSON.parse(text) : null };
  }, { path, method, body });
}
async function read(page, path) { const result = await request(page, path); expect(result.status).toBe(200); return result.body; }
async function uiResponse(page, method, suffix, status, action) {
  const pending = page.waitForResponse(r => r.request().method() === method && new URL(r.url()).pathname.endsWith(suffix));
  await action(); const response = await pending; expect(response.status()).toBe(status); return response.json();
}
async function confirm(page, text, action) {
  const pending = page.waitForEvent('dialog');
  const clicked = action(); const dialog = await pending;
  expect(dialog.type()).toBe('confirm'); expect(dialog.message()).toBe(text); await dialog.accept(); await clicked;
}
async function open(page, name = 'Workshop geometry') {
  await uiResponse(page, 'GET', '/graph', 200, () => button(page, name).click());
  await expect(page.getByRole('heading', { name: 'Collaborative authoring', exact: true })).toBeVisible();
}
async function mapped(page, title) {
  await page.getByLabel('New node kind').selectOption('outcome');
  await page.getByLabel('Name', { exact: true }).fill(title);
  await expect(button(page, 'Add node')).toBeDisabled();
  await page.getByLabel('Standards catalog').selectOption({ label: 'Framework 7' });
  await page.getByLabel('Mapped standard').selectOption({ label: standard });
  await uiResponse(page, 'POST', '/nodes', 201, () => button(page, 'Add node').click());
  await expect(node(page, title)).toBeVisible();
}
async function approve(page) {
  await uiResponse(page, 'GET', '/graph', 200, () => button(page, 'Reload plan and review').click());
  await expect(button(page, 'Approve this snapshot')).toBeEnabled();
  const result = await uiResponse(page, 'POST', '/approval', 200, () => button(page, 'Approve this snapshot').click());
  expect(result.state).toBe('approved'); expect(result.reviewerName).toBe('Ruth Reviewer');
  await expect(page.getByRole('region', { name: 'Draft review' }).getByText('approved — Ruth Reviewer', { exact: true })).toBeVisible();
  return result;
}
async function publish(page, title, status) {
  return uiResponse(page, 'POST', '/publish', status, () => confirm(page, `Publish ${title}? This freezes the revision.`, () => button(page, 'Publish').click()));
}
async function policy(page, sharing = true) {
  await page.getByLabel('Require current reviewer approval to publish').check();
  await page.getByLabel('Enable read-only sharing').setChecked(sharing);
  const result = await uiResponse(page, 'PUT', '/collaboration-policy', 200, () => confirm(page, sharing ? 'Apply this collaboration policy?' : 'Disable sharing and revoke every outgoing workspace grant?', () => button(page, 'Save policy').click()));
  expect(result.requireApprovalForPublish).toBe(true); expect(result.sharingEnabled).toBe(sharing);
}
async function share(page, workspace) {
  await page.getByLabel('Target workspace ID (provided by its owner)').fill(workspace);
  await uiResponse(page, 'POST', '/shares', 200, () => confirm(page, `Share this curriculum read-only with workspace ${workspace}?`, () => button(page, 'Grant read-only access').click()));
  await expect(page.getByText('Co-op Readers', { exact: true })).toBeVisible();
}
async function template(page, name) {
  await page.getByLabel('Template', { exact: true }).selectOption({ label: 'Workshop starter' });
  await page.getByLabel('Curriculum name').fill(name);
  const created = await uiResponse(page, 'POST', '/curricula', 201, () => button(page, 'Create from template').click());
  await expect(node(page, 'Craftsmanship')).toBeVisible(); await expect(node(page, 'First workshop')).toBeVisible();
  return { id: created.id, revision: await revisionSelect(page).inputValue() };
}
async function diff(page, baseline = 'Workshop geometry — baseline · draft') {
  const result = await uiResponse(page, 'GET', '/diff', 200, () => page.getByLabel('Compare from').selectOption({ label: baseline }));
  expect(result.addedOutcomes.map(o => o.name)).toContain('Draw a scale plan');
  expect(result.removedOutcomes.map(o => o.name)).toContain('Read a scale');
  const region = page.getByRole('region', { name: 'Revision comparison' });
  await expect(region.getByText('Draw a scale plan', { exact: true })).toBeVisible(); await expect(region.getByText('Read a scale', { exact: true })).toBeVisible();
}
async function themes(page, info, prefix) {
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.screenshot({ path: info.outputPath(`${prefix}-dark.png`), fullPage: true });
  await button(page, 'Switch to light theme').click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(node(page, 'Draw a scale plan')).toBeVisible();
  await page.screenshot({ path: info.outputPath(`${prefix}-light.png`), fullPage: true });
  await button(page, 'Switch to dark theme').click();
}

test('S17 real multi-principal collaboration and policy publication', async ({ browserName }, info) => {
  expect(browserName).toBe('chromium'); // Option fixture only; does not launch a browser.
  info.setTimeout(180_000); // Full fixture build/container + multi-persona journey budget; no retries.
  const host = await startS17Fixture(studioRoot, console.log);
  // Launch after Docker has established the disposable network, not before.
  // Otherwise its route-change notification interrupts Chromium font requests.
  let browser;
  const contexts = [], audits = [];
  async function session(persona, mobile = false) {
    const context = await browser.newContext({ baseURL: host.url, ...(mobile ? { viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true } : { viewport: { width: 1440, height: 1000 } }) });
    contexts.push(context); const page = await context.newPage(); const errors = audit(page); audits.push(errors);
    await page.goto('/_fixture/'); await page.getByLabel('Persona').selectOption({ label: persona });
    await button(page, 'Start fixture session').click(); await expect(page.getByText('Validated workspace membership')).toBeVisible();
    return { page, errors, context };
  }
  try {
    browser = await chromium.launch({ channel: 'chromium' }); // Full headless browser, like MCP; not headless-shell.
    const ada = await session('Ada Author (author)'), ruth = await session('Ruth Reviewer (reviewer)'), owner = await session('Olivia Owner (owner)'), w2 = await session('Co-op Author (author)'), w3 = await session('Unshared Author (author)');
    const ids = await read(ada.page, '/_fixture/evidence');
    const base = `/studio/v1/revisions/${ids.draftRevisionId}`;
    const aMe = await read(ada.page, '/studio/v1/auth/me'), rMe = await read(ruth.page, '/studio/v1/auth/me');
    expect(aMe).not.toEqual(rMe); expect(ada.context).not.toBe(ruth.context);
    await expect(ada.page.getByLabel('Active workspace')).toContainText('Workshop Authors · author');
    await expect(ruth.page.getByLabel('Active workspace')).toContainText('Workshop Authors · reviewer');

    await test.step('Diff and durable named node/item comments across independent reload', async () => {
      await open(ruth.page); await diff(ruth.page); await open(ada.page);
      for (const target of ['Draw a scale plan', 'Scale drawing practice']) {
        if (target === 'Scale drawing practice') await ada.page.getByLabel('Materialization run').selectOption(ids.runId);
        const opener = button(ada.page, target === 'Draw a scale plan' ? `Comments on ${target}` : `Comment on ${target}`);
        await opener.click(); const drawer = ada.page.getByRole('dialog', { name: target });
        await expect(drawer.getByText('No comments yet.')).toBeVisible(); await expect(button(drawer, 'Close comments')).toBeFocused();
        await drawer.getByLabel('Comment', { exact: true }).fill(`Durable ${target} note from Ada.`);
        const refreshed = ada.page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname.endsWith('/comments'));
        const saved = await uiResponse(ada.page, 'POST', '/comments', 201, () => button(drawer, 'Post comment').click());
        expect(await (await refreshed).finished()).toBeNull();
        expect(saved.authorName).toBe('Ada Author');
        await expect(drawer.getByText(saved.body, { exact: true })).toBeVisible();
        await ada.page.keyboard.press('Escape'); await expect(drawer).toHaveCount(0); await expect(opener).toBeFocused();
        await ruth.page.reload(); await open(ruth.page);
        if (target === 'Scale drawing practice') await ruth.page.getByLabel('Materialization run').selectOption(ids.runId);
        await button(ruth.page, target === 'Draw a scale plan' ? `Comments on ${target}` : `Comment on ${target}`).click();
        const durable = ruth.page.getByRole('dialog', { name: target });
        await expect(durable.getByText('Ada Author', { exact: true })).toBeVisible(); await expect(durable.getByText(saved.body, { exact: true })).toBeVisible();
        await button(durable, 'Close comments').click(); await expect(durable).toHaveCount(0);
      }
    });
    await test.step('Named template and saved/imported library preserve populated graph', async () => {
      await ada.page.getByLabel('Unit to save').selectOption({ label: 'Workshop geometry unit' });
      await ada.page.getByLabel('Library entry name').fill('Browser saved workshop');
      await uiResponse(ada.page, 'POST', '/unit-library', 201, () => button(ada.page, 'Save unit to library').click());
      const t = await template(owner.page, 'Browser template curriculum');
      await uiResponse(owner.page, 'POST', `/unit-library/${ids.libraryId}`, 201, () => button(owner.page, 'Import Reusable workshop geometry').click());
      await expect(node(owner.page, 'Workshop geometry unit')).toBeVisible(); await expect(node(owner.page, 'Draw a scale plan')).toBeVisible();
      const graph = await read(owner.page, `/studio/v1/revisions/${t.revision}/graph`);
      expect(graph.nodes.map(n => n.title)).toEqual(expect.arrayContaining(['Craftsmanship', 'First workshop', 'Workshop geometry unit', 'Draw a scale plan'])); expect(graph.edges.length).toBeGreaterThan(0);
    });
    await test.step('Actual mapped stale snapshot, approval policy denial, current approval then publish200', async () => {
      const oldGraph = await read(ruth.page, `${base}/graph`), oldReview = await read(ruth.page, `${base}/approval`);
      expect(oldGraph.contentFingerprint).toBe(oldReview.contentFingerprint);
      await expect(button(ada.page, 'Approve this snapshot')).toHaveCount(0);
      ada.errors.allow('author cannot forge approval', 'POST', `${base}/approval`, 403);
      expect((await request(ada.page, `${base}/approval`, 'POST', { decision: 'approved', contentFingerprint: oldReview.contentFingerprint })).status).toBe(403);
      await mapped(ada.page, 'Browser mapped stale outcome');
      ruth.errors.allow('old graph fingerprint', 'POST', `${base}/approval`, 409);
      await uiResponse(ruth.page, 'POST', '/approval', 409, () => button(ruth.page, 'Approve this snapshot').click());
      await expect(ruth.page.getByText('conflict Reload the current review before deciding again.')).toBeVisible(); await approve(ruth.page);
      await policy(owner.page); await mapped(ada.page, 'Browser mapped policy outcome');
      ada.errors.allow('valid graph needs current approval', 'POST', `${base}/publish`, 409);
      const denied = await publish(ada.page, 'Workshop geometry — draft', 409); expect(denied.detail).toBe('current reviewer approval required');
      await expect(ada.page.getByRole('status').filter({ hasText: 'current reviewer approval required' })).toBeVisible();
      const approval = await approve(ruth.page); const current = await read(ada.page, `${base}/graph`); expect(approval.contentFingerprint).toBe(current.contentFingerprint);
      const result = await publish(ada.page, 'Workshop geometry — draft', 200); expect(result.state).toBe('published');
      await expect(revisionSelect(ada.page).locator('option:checked')).toHaveText('Workshop geometry — draft · published'); await expect(button(ada.page, 'Publish')).toBeDisabled();
      const persisted = await read(ada.page, base); expect(persisted.state).toBe('published'); expect(persisted.publishedAt).toBeTruthy();
      await info.attach('published-state', { body: JSON.stringify({ state: persisted.state, revision: persisted.id, approval: approval.state }), contentType: 'application/json' });
      await ada.page.reload(); await open(ada.page); await expect(revisionSelect(ada.page).locator('option:checked')).toContainText('published'); await expect(button(ada.page, 'Publish')).toBeDisabled();
      await diff(ada.page); await themes(ada.page, info, 'desktop');
    });
    await test.step('Approved invalid draft still fails graph validation; shared draft denies valid mutations', async () => {
      const t = await template(owner.page, 'Browser invalid and share curriculum'); const draft = `/studio/v1/revisions/${t.revision}`;
      const unit = { kind: 'unit', title: 'W2 authorization probe', position: 0 };
      expect((await request(ada.page, `${draft}/nodes`, 'POST', { ...unit, title: 'Author control unit' })).status).toBe(201);
      await share(owner.page, ids.workspaces.w2);
      await w2.page.reload(); await open(w2.page, 'Browser invalid and share curriculum'); await expect(node(w2.page, 'Author control unit')).toBeVisible();
      const before = await read(ada.page, `${draft}/graph`);
      w2.errors.allow('shared draft valid unit denied', 'POST', `${draft}/nodes`, 404);
      expect((await request(w2.page, `${draft}/nodes`, 'POST', unit)).status).toBe(404);
      const after = await read(ada.page, `${draft}/graph`); expect(after).toEqual(before); expect(after.contentFingerprint).toMatch(/^[a-f0-9]{32}$/); expect(after.nodes.map(n => n.title)).not.toContain(unit.title);
      expect((await request(ada.page, `${draft}/nodes`, 'POST', { kind: 'outcome', title: 'Intentionally unmapped outcome', position: 1 })).status).toBe(201);
      await ruth.page.reload(); await open(ruth.page, 'Browser invalid and share curriculum'); await approve(ruth.page);
      await ada.page.reload(); await open(ada.page, 'Browser invalid and share curriculum');
      ada.errors.allow('invalid but approved graph denied', 'POST', `${draft}/publish`, 409);
      const denied = await publish(ada.page, 'Browser invalid and share curriculum — draft 1', 409);
      expect(denied.detail).toContain('Revision validation failed'); expect(JSON.stringify(denied)).toContain('OUTCOME_UNMAPPED');
      await expect(ada.page.getByRole('status').filter({ hasText: /Revision validation failed.*OUTCOME_UNMAPPED/ })).toBeVisible(); expect((await read(ada.page, draft)).state).toBe('draft');
    });
    await test.step('Explicit shared graph/diff, protected private actions, W3 denial, no grant resurrection', async () => {
      await open(owner.page); await share(owner.page, ids.workspaces.w2); await w2.page.reload(); await open(w2.page); await diff(w2.page);
      await expect(w2.page.getByText('Shared read-only curriculum. Comments, review, exports and editing are unavailable.')).toBeVisible();
      await expect(button(w2.page, 'Comments on Draw a scale plan')).toHaveCount(0); await expect(button(w2.page, 'Publish')).toBeDisabled();
      await expect(w3.page.getByText('NO CURRICULA FOUND')).toBeVisible();
      const graph = await read(ada.page, `${base}/graph`); const target = graph.nodes.find(n => n.title === 'Draw a scale plan');
      const probes = [
        ['GET', `${base}/comments`], ['POST', `${base}/comments`, { nodeId: target.id, body: 'Denied private comment' }],
        ['GET', `${base}/approval`], ['POST', `${base}/approval`, { decision: 'approved', contentFingerprint: graph.contentFingerprint }],
        ['GET', `/studio/v1/materialized-items/${ids.itemId}/comments`], ['POST', `/studio/v1/materialized-items/${ids.itemId}/comments`, { body: 'Denied item comment' }],
      ];
      for (const who of [w2, w3]) for (const [method, path, body] of probes) { who.errors.allow('private action denied', method, path, 404); expect((await request(who.page, path, method, body)).status).toBe(404); }
      w3.errors.allow('unshared graph denied', 'GET', `${base}/graph`, 404); expect((await request(w3.page, `${base}/graph`)).status).toBe(404);
      await policy(owner.page, false); await w2.page.reload(); await expect(w2.page.getByText('NO CURRICULA FOUND')).toBeVisible();
      await policy(owner.page, true); await w2.page.reload(); await expect(w2.page.getByText('NO CURRICULA FOUND')).toBeVisible();
      w2.errors.allow('re-enable cannot restore revoked grant', 'GET', `${base}/graph`, 404); expect((await request(w2.page, `${base}/graph`)).status).toBe(404);
      expect((await read(owner.page, `/studio/v1/curricula/${ids.curriculumId}/shares?limit=10&offset=0`)).totalCount).toBe(0);
    });
    await test.step('Mobile actual tap, no horizontal overflow, diff, modal focus, dark/light parity', async () => {
      const mobile = await session('Ada Author (author)', true); const page = mobile.page; const opener = button(page, 'Workshop geometry');
      await expect(opener).toBeVisible(); const geometry = await opener.evaluate(el => ({ layout: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth, visual: visualViewport.width, offset: visualViewport.offsetTop, target: el.getBoundingClientRect().height, dpr: devicePixelRatio }));
      expect(geometry).toEqual({ layout: 390, scroll: 390, visual: 390, offset: 0, target: 44, dpr: 3 });
      await opener.tap(); await expect(page.getByRole('heading', { name: 'Collaborative authoring', exact: true })).toBeVisible(); await diff(page);
      const comments = button(page, 'Comments on Draw a scale plan'); await comments.tap(); const drawer = page.getByRole('dialog', { name: 'Draw a scale plan' });
      await expect(button(drawer, 'Close comments')).toBeFocused(); await expect(drawer.getByText('Ada Author', { exact: true })).toBeVisible();
      await page.screenshot({ path: info.outputPath('mobile-drawer-dark.png') }); await page.keyboard.press('Escape'); await expect(drawer).toHaveCount(0); await expect(comments).toBeFocused();
      await themes(page, info, 'mobile');
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
    });
    for (const entry of audits) entry.check();
  } finally {
    try { await Promise.all(contexts.map(c => c.close())); await browser?.close(); } finally { await stopS17Fixture(host, console.log); }
  }
});

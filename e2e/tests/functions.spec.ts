import { test, expect, Page } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

async function goToDashboard(page: Page) {
  await loginAsAdmin(page);
  const nodes: Array<{ id: number }> = await page.evaluate(async () => {
    const r = await fetch('/api/nodes');
    return r.json();
  });
  if (nodes.length > 0) {
    await page.goto(`/dashboard/${nodes[0].id}`);
  }
}

test.describe('Functions menu', () => {
  test.beforeEach(async ({ page }) => {
    await goToDashboard(page);
  });

  test('Functions button is present', async ({ page }) => {
    await expect(page.locator('#functions-btn')).toBeVisible();
  });

  test('clicking Functions button opens the dropdown', async ({ page }) => {
    await page.locator('#functions-btn').click();
    await expect(page.locator('#functions-menu')).toBeVisible();
  });

  test('menu contains at least one command', async ({ page }) => {
    await page.locator('#functions-btn').click();
    await expect(page.locator('#functions-menu .dropdown-item').first()).toBeVisible();
  });

  test('menu has group headers as separators', async ({ page }) => {
    await page.locator('#functions-btn').click();
    await expect(page.locator('#functions-menu .dropdown-header').first()).toBeVisible();
  });

  test('group headers are capitalized', async ({ page }) => {
    await page.locator('#functions-btn').click();
    const headers = page.locator('#functions-menu .dropdown-header');
    const count = await headers.count();
    // Skip the first "Functions" title header; check subsequent group headers.
    for (let i = 1; i < count; i++) {
      const text = await headers.nth(i).textContent();
      if (text && text.trim().length > 0) {
        expect(text.trim()[0]).toBe(text.trim()[0].toUpperCase());
      }
    }
  });

  test('API returns commands for node', async ({ page }) => {
    const nodes: Array<{ id: number }> = await page.evaluate(async () => {
      const r = await fetch('/api/nodes');
      return r.json();
    });
    if (nodes.length === 0) return;
    const cmds: Array<{ index: number; name: string; check: string }> = await page.evaluate(
      async (id) => {
        const r = await fetch(`/api/nodes/${id}/commands`);
        return r.json();
      },
      nodes[0].id,
    );
    expect(Array.isArray(cmds)).toBe(true);
    expect(cmds.length).toBeGreaterThan(0);
    expect(cmds[0]).toHaveProperty('index');
    expect(cmds[0]).toHaveProperty('name');
    expect(cmds[0]).toHaveProperty('check');
  });

  test('command check string is 16 hex characters', async ({ page }) => {
    const nodes: Array<{ id: number }> = await page.evaluate(async () => {
      const r = await fetch('/api/nodes');
      return r.json();
    });
    if (nodes.length === 0) return;
    const cmds: Array<{ check: string }> = await page.evaluate(async (id) => {
      const r = await fetch(`/api/nodes/${id}/commands`);
      return r.json();
    }, nodes[0].id);
    for (const cmd of cmds) {
      expect(cmd.check).toMatch(/^[0-9a-f]{16}$/);
    }
  });

  test('POST with bad check returns 400', async ({ page }) => {
    const nodes: Array<{ id: number }> = await page.evaluate(async () => {
      const r = await fetch('/api/nodes');
      return r.json();
    });
    if (nodes.length === 0) return;
    const status: number = await page.evaluate(async (id) => {
      const r = await fetch(`/api/nodes/${id}/cmd`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ index: 0, check: 'badcheckstring0', args: {} }),
      });
      return r.status;
    }, nodes[0].id);
    expect(status).toBe(400);
  });

  test('POST with out-of-range index returns 400', async ({ page }) => {
    const nodes: Array<{ id: number }> = await page.evaluate(async () => {
      const r = await fetch('/api/nodes');
      return r.json();
    });
    if (nodes.length === 0) return;
    const status: number = await page.evaluate(async (id) => {
      const r = await fetch(`/api/nodes/${id}/cmd`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ index: 9999, check: 'aaaaaaaaaaaaaaaa', args: {} }),
      });
      return r.status;
    }, nodes[0].id);
    expect(status).toBe(400);
  });

  test('Show Uptime command shows output modal', async ({ page }) => {
    await page.locator('#functions-btn').click();
    const uptimeBtn = page.locator('#functions-menu .dropdown-item', { hasText: 'Show Uptime' });
    // Only run if the command is visible (AMI may be offline in some test envs).
    if (await uptimeBtn.isVisible()) {
      await uptimeBtn.click();
      await expect(page.locator('#cmdOutputModal')).toBeVisible({ timeout: 8000 });
      await expect(page.locator('#cmd-output-body')).not.toBeEmpty();
    }
  });
});

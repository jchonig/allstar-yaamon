import { test, expect } from '@playwright/test';
import { loginAsAdmin, loginAsViewer } from './fixtures';

test.describe('Graph page', () => {
  test('spinner hides and SVG renders after load', async ({ page }) => {
    await loginAsAdmin(page);
    // Navigate directly to graph page for the test node.
    await page.goto('/graph/99999');

    // Spinner must disappear (d-none added by JS when data arrives).
    const spinner = page.locator('#graph-loading');
    await expect(spinner).not.toBeVisible({ timeout: 10_000 });

    // D3 must have rendered at least one node group inside the SVG.
    const svgNodes = page.locator('#graph-svg g g');
    await expect(svgNodes.first()).toBeAttached({ timeout: 5_000 });
  });

  test('shows node number in heading', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/graph/99999');
    await expect(page.locator('h5 code')).toContainText('99999');
  });

  test('filter input is present', async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/graph/99999');
    await expect(page.locator('#graph-filter')).toBeVisible();
  });

  test('viewer can view graph page', async ({ page }) => {
    await loginAsViewer(page);
    await page.goto('/graph/99999');
    // Page should load (not redirect to login) — viewer has readonly access.
    await expect(page.locator('h5.mb-0')).toContainText('Network Graph', { timeout: 10_000 });
  });

  test('d3.min.js served locally', async ({ page }) => {
    await loginAsAdmin(page);
    const d3Request = page.waitForRequest(req => req.url().includes('d3.min.js'));
    await page.goto('/graph/99999');
    const req = await d3Request;
    // Must come from the app server, not an external CDN.
    expect(req.url()).toContain('d3.min.js');
    expect(req.url()).not.toContain('cdn.jsdelivr.net');
    expect(req.url()).not.toContain('unpkg.com');
  });
});

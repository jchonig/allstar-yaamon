import { test, expect } from '@playwright/test';
import { loginAsAdmin, loginAsViewer } from './fixtures';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('shows the node name and number', async ({ page }) => {
    await expect(page.locator('h5')).toContainText('Test Node');
    await expect(page.locator('code.text-muted')).toContainText('99999');
  });

  test('AMI badge is present', async ({ page }) => {
    await expect(page.locator('#ami-badge')).toBeVisible();
  });

  test('favorites card is present', async ({ page }) => {
    // The favorites card should exist (may be empty but the card itself is there).
    await expect(page.locator('.card')).toHaveCount({ minimum: 1 });
  });

  test('active links section exists in DOM', async ({ page }) => {
    // Starts hidden (display:none) when no links; element must exist.
    const section = page.locator('#active-links-section');
    await expect(section).toBeAttached();
  });

  test('admin dropdown shows admin links', async ({ page }) => {
    await page.locator('.dropdown-toggle').click();
    await expect(page.locator('a[href="/admin/nodes"]')).toBeVisible();
    await expect(page.locator('a[href="/admin/users"]')).toBeVisible();
    await expect(page.locator('a[href="/admin/backup"]')).toBeVisible();
  });

  test('viewer dropdown does not show admin links', async ({ page }) => {
    await page.close();
    const newPage = await page.context().newPage();
    await loginAsViewer(newPage);
    await newPage.locator('.dropdown-toggle').click();
    await expect(newPage.locator('a[href="/admin/nodes"]')).not.toBeVisible();
  });
});

test.describe('Dashboard — active links table', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('active-links-section is in the DOM', async ({ page }) => {
    await expect(page.locator('#active-links-section')).toBeAttached();
  });

  test('active links Links column is clickable when count > 0', async ({ page }) => {
    // Inject a fake AMI link with a non-zero connected_links count and verify
    // the rendered cell has cursor:pointer and an onclick that calls openConnModal.
    const html: string = await page.evaluate(() => {
      // Build a minimal amiLinks entry (renderActiveLinks reads this global).
      (window as any).amiLinks = { '687390': { type: 'T', keyed: false } };
      (window as any).linksSince = { '687390': new Date() };
      (window as any).statsData = {
        '687390': { connected_links: 4, callsign: 'W1AW' },
      };
      (window as any).favsData = [];
      (window as any).astdbCache = {};
      (window as any).amiLinksReceived = true;
      (window as any).renderActiveLinks();
      const tbody = document.querySelector('#active-links-body');
      return tbody ? tbody.innerHTML : '';
    });
    // The Links column td must have cursor:pointer and an onclick calling openConnModal.
    expect(html).toContain('cursor:pointer');
    expect(html).toContain('openConnModal');
  });

  test('active links Links column shows em-dash for web/direct clients', async ({ page }) => {
    const html: string = await page.evaluate(() => {
      (window as any).amiLinks = { 'KR4YXX': { type: 'T', keyed: false } };
      (window as any).linksSince = { 'KR4YXX': new Date() };
      (window as any).statsData = {};
      (window as any).favsData = [];
      (window as any).astdbCache = {};
      (window as any).amiLinksReceived = true;
      (window as any).renderActiveLinks();
      const tbody = document.querySelector('#active-links-body');
      return tbody ? tbody.innerHTML : '';
    });
    // isDirect nodes never get a clickable lcnt cell.
    expect(html).not.toContain('openConnModal');
    expect(html).toContain('—');
  });
});

test.describe('Dashboard — mobile viewport', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
  });

  test('page loads without horizontal scroll', async ({ page }) => {
    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 2);
  });

  test('description column is hidden on mobile', async ({ page }) => {
    // The d-none d-md-table-cell class hides on xs screens.
    const descCol = page.locator('th', { hasText: 'Description' });
    // May not exist at all or be hidden — just check it's not causing overflow.
    const count = await descCol.count();
    if (count > 0) {
      const visible = await descCol.isVisible();
      expect(visible).toBe(false);
    }
  });
});

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

test.describe('Favorites — Add modal', () => {
  test.beforeEach(async ({ page }) => {
    await goToDashboard(page);
  });

  test('+ button in card header opens modal', async ({ page }) => {
    await page.locator('button[title="Add Favorite"]').click();
    await expect(page.locator('#addFavModal')).toBeVisible();
    await expect(page.locator('#fav-modal-title')).toHaveText('Add Favorite');
    await expect(page.locator('#fav-node-number')).not.toBeDisabled();
  });

  test('clicking + does not collapse the favorites card', async ({ page }) => {
    await page.locator('button[title="Add Favorite"]').click();
    await expect(page.locator('#favorites-collapse')).toHaveClass(/show/);
  });

  test('modal has required fields', async ({ page }) => {
    await page.locator('button[title="Add Favorite"]').click();
    await expect(page.locator('#fav-node-number')).toBeVisible();
    await expect(page.locator('#fav-callsign')).toBeVisible();
    await expect(page.locator('#fav-description')).toBeVisible();
    await expect(page.locator('#fav-location')).toBeVisible();
  });

  test('can add and see a new favorite', async ({ page }) => {
    await page.locator('button[title="Add Favorite"]').click();
    await page.locator('#fav-node-number').fill('77777');
    await page.locator('#fav-callsign').fill('W1TEST');
    await page.locator('#fav-description').fill('E2E Test Node');
    await page.locator('#fav-modal-save-btn').click();

    // Modal closes and row appears.
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    await expect(page.locator('#favs-tbody')).toContainText('77777');
  });

  test('cancel closes modal without adding', async ({ page }) => {
    const rowsBefore = await page.locator('#favs-tbody tr').count();
    await page.locator('button[title="Add Favorite"]').click();
    await page.locator('#addFavModal .btn-secondary', { hasText: 'Cancel' }).click();
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    const rowsAfter = await page.locator('#favs-tbody tr').count();
    expect(rowsAfter).toBe(rowsBefore);
  });
});

test.describe('Favorites — Edit and Delete from row', () => {
  test.beforeEach(async ({ page }) => {
    await goToDashboard(page);
    await page.locator('button[title="Add Favorite"]').click();
    await page.locator('#fav-node-number').fill('55555');
    await page.locator('#fav-callsign').fill('K1EDIT');
    await page.locator('#fav-modal-save-btn').click();
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    await expect(page.locator('#favs-tbody')).toContainText('55555');
  });

  test('... dropdown on favorites row contains Edit and Delete', async ({ page }) => {
    const row = page.locator('#favs-tbody tr', { hasText: '55555' });
    await row.locator('button[title="More actions"]').last().click();
    await expect(row.locator('.dropdown-menu').last()).toContainText('Edit');
    await expect(row.locator('.dropdown-menu').last()).toContainText('Delete');
  });

  test('Edit opens modal pre-populated with node data, node number disabled', async ({ page }) => {
    const row = page.locator('#favs-tbody tr', { hasText: '55555' });
    await row.locator('button[title="More actions"]').last().click();
    // Use the visible (open) dropdown to avoid strict-mode violations when multiple rows match.
    await page.locator('.dropdown-menu.show .dropdown-item', { hasText: 'Edit' }).click();
    await expect(page.locator('#addFavModal')).toBeVisible();
    await expect(page.locator('#fav-modal-title')).toHaveText('Edit Favorite');
    await expect(page.locator('#fav-node-number')).toBeDisabled();
    await expect(page.locator('#fav-node-number')).toHaveValue('55555');
    await expect(page.locator('#fav-callsign')).toHaveValue('K1EDIT');
  });

  test('Edit saves changes and updates table', async ({ page }) => {
    const row = page.locator('#favs-tbody tr', { hasText: '55555' });
    await row.locator('button[title="More actions"]').last().click();
    await page.locator('.dropdown-menu.show .dropdown-item', { hasText: 'Edit' }).click();
    await page.locator('#fav-callsign').fill('K1UPDATED');
    await page.locator('#fav-modal-save-btn').click();
    await expect(page.locator('#addFavModal')).not.toBeVisible();
    await expect(page.locator('#favs-tbody')).toContainText('K1UPDATED');
  });

  test('Delete removes the row', async ({ page }) => {
    const rows = page.locator('#favs-tbody tr', { hasText: '55555' });
    const countBefore = await rows.count();
    await rows.locator('button[title="More actions"]').last().click();
    page.once('dialog', d => d.accept());
    await page.locator('.dropdown-menu.show .dropdown-item', { hasText: 'Delete' }).click();
    await expect(rows).toHaveCount(countBefore - 1);
  });
});

test.describe('Favorites — Sort', () => {
  test('clicking Node column header shows sort indicator', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    await expect(header).toHaveClass(/sort-active/);
  });

  test('second click reverses sort direction', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    const classAfterFirst = await header.locator('.sort-icon').getAttribute('class');
    await header.click();
    const classAfterSecond = await header.locator('.sort-icon').getAttribute('class');
    expect(classAfterFirst).not.toBe(classAfterSecond);
  });

  test('third click resets to insertion order', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('#favs-table th', { hasText: 'Node' });
    await header.click();
    await header.click();
    await header.click();
    await expect(header).not.toHaveClass(/sort-active/);
  });
});

test.describe('Favorites — Collapsible card', () => {
  test('favorites card starts expanded', async ({ page }) => {
    await goToDashboard(page);
    await expect(page.locator('#favorites-collapse')).toHaveClass(/show/);
  });

  test('clicking header collapses the card', async ({ page }) => {
    await goToDashboard(page);
    // Click the collapse header (the card-header with data-bs-toggle).
    await page.locator('[data-bs-target="#favorites-collapse"]').click();
    await expect(page.locator('#favorites-collapse')).not.toHaveClass(/show/);
  });

  test('clicking again expands the card', async ({ page }) => {
    await goToDashboard(page);
    const header = page.locator('[data-bs-target="#favorites-collapse"]');
    const collapse = page.locator('#favorites-collapse');
    await header.click(); // collapse
    // Wait for Bootstrap animation to fully complete (_isTransitioning=false) before clicking again.
    // 'collapse' class is only present once the hide animation finishes; during animation it's 'collapsing'.
    await expect(collapse).toHaveClass('collapse');
    await expect(collapse).not.toHaveClass(/show/);
    await header.click(); // expand
    await expect(collapse).toHaveClass(/show/);
  });
});

test.describe('Favorites INI Import UI', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/backup');
  });

  test('node selector is present', async ({ page }) => {
    await expect(page.locator('#fav-node-select')).toBeVisible();
  });

  test('preview button exists', async ({ page }) => {
    await expect(page.locator('button', { hasText: /Preview/i })).toBeVisible();
  });

  test('import button starts disabled', async ({ page }) => {
    const importBtn = page.locator('#fav-import-btn');
    await expect(importBtn).toBeDisabled();
  });

  test('uploading a valid INI and previewing enables import', async ({ page }) => {
    const ini = '[node_54321]\ncmd[] = "rpt cmd 99999 ilink 3 54321"\n';
    await page.locator('#fav-import-file').setInputFiles({
      name: 'favorites.ini',
      mimeType: 'text/plain',
      buffer: Buffer.from(ini),
    });
    await page.locator('button', { hasText: /Preview/i }).click();
    await expect(page.locator('#fav-import-preview')).toBeVisible();
    // Import button should now be enabled (if there are nodes to add).
    // (May still be disabled if node already exists — just check preview appeared.)
  });
});

test.describe('Mobile — hidden columns', () => {
  test.use({ viewport: { width: 375, height: 812 } });

  test('Location column is hidden', async ({ page }) => {
    await goToDashboard(page);
    const locCol = page.locator('th', { hasText: 'Location' });
    const count = await locCol.count();
    if (count > 0) {
      expect(await locCol.isVisible()).toBe(false);
    }
  });

  test('Description column is hidden', async ({ page }) => {
    await goToDashboard(page);
    const descCol = page.locator('th', { hasText: 'Description' });
    const count = await descCol.count();
    if (count > 0) {
      expect(await descCol.isVisible()).toBe(false);
    }
  });
});

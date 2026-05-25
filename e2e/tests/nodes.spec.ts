import { test, expect } from '@playwright/test';
import { loginAsAdmin } from './fixtures';

test.describe('Admin — Node modals', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page);
    await page.goto('/admin/nodes');
  });

  test('Add Node modal opens with empty form', async ({ page }) => {
    await page.locator('button', { hasText: /Add Node/i }).click();
    await expect(page.locator('#nodeModal')).toBeVisible();
    await expect(page.locator('#node-name')).toHaveValue('');
    await expect(page.locator('#node-number')).toHaveValue('');
  });

  test('can create a node and see it in the table', async ({ page }) => {
    await page.locator('button', { hasText: /Add Node/i }).click();
    await page.locator('#node-name').fill('E2E Test Node');
    await page.locator('#node-number').fill('11111');
    await page.locator('#node-ami-host').fill('localhost');
    await page.locator('#node-ami-port').fill('5038');
    await page.locator('#node-save-btn').click();

    await expect(page.locator('#nodeModal')).not.toBeVisible();
    await expect(page.locator('#nodes-tbody')).toContainText('E2E Test Node');
    await expect(page.locator('#nodes-tbody')).toContainText('11111');
  });

  test('Edit button opens modal pre-populated with node data', async ({ page }) => {
    // Find the seeded Test Node row and click its Edit button.
    const row = page.locator('#nodes-tbody tr', { hasText: 'Test Node' });
    await row.locator('button', { hasText: 'Edit' }).click();
    await expect(page.locator('#nodeModal')).toBeVisible();
    await expect(page.locator('#node-name')).toHaveValue('Test Node');
    await expect(page.locator('#node-number')).toHaveValue('99999');
  });

  test('Delete modal appears when Delete is clicked', async ({ page }) => {
    // Create a node to delete.
    await page.locator('button', { hasText: /Add Node/i }).click();
    await page.locator('#node-name').fill('Delete Me');
    await page.locator('#node-number').fill('88881');
    await page.locator('#node-save-btn').click();
    await expect(page.locator('#nodes-tbody')).toContainText('Delete Me');

    const row = page.locator('#nodes-tbody tr', { hasText: 'Delete Me' });
    await row.locator('button', { hasText: 'Delete' }).click();
    await expect(page.locator('#deleteModal')).toBeVisible();
    await expect(page.locator('#delete-node-name')).toContainText('Delete Me');
  });

  test('confirming delete removes the node', async ({ page }) => {
    // Create then delete.
    await page.locator('button', { hasText: /Add Node/i }).click();
    await page.locator('#node-name').fill('Temporary Node');
    await page.locator('#node-number').fill('88882');
    await page.locator('#node-save-btn').click();
    await expect(page.locator('#nodes-tbody')).toContainText('Temporary Node');

    const row = page.locator('#nodes-tbody tr', { hasText: 'Temporary Node' });
    await row.locator('button', { hasText: 'Delete' }).click();
    await page.locator('#deleteModal button', { hasText: 'Delete' }).click();
    await expect(page.locator('#nodes-tbody')).not.toContainText('Temporary Node');
  });

  test('cancel Add Node does not create a node', async ({ page }) => {
    const countBefore = await page.locator('#nodes-tbody tr').count();
    await page.locator('button', { hasText: /Add Node/i }).click();
    await page.locator('#node-name').fill('Should Not Appear');
    await page.locator('#nodeModal .btn-secondary', { hasText: 'Cancel' }).click();
    await expect(page.locator('#nodeModal')).not.toBeVisible();
    const countAfter = await page.locator('#nodes-tbody tr').count();
    expect(countAfter).toBe(countBefore);
  });
});

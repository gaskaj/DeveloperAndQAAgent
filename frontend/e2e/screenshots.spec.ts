import { test, expect } from '@playwright/test';

/**
 * Generates a test email in the format:
 * WebTesting-YYYYMMDD-HHMMSSUTC-GUID@OpenAgentFramework.com
 */
function generateTestEmail(): string {
  const now = new Date();
  const yyyy = now.getUTCFullYear().toString();
  const mm = (now.getUTCMonth() + 1).toString().padStart(2, '0');
  const dd = now.getUTCDate().toString().padStart(2, '0');
  const hh = now.getUTCHours().toString().padStart(2, '0');
  const min = now.getUTCMinutes().toString().padStart(2, '0');
  const ss = now.getUTCSeconds().toString().padStart(2, '0');
  const guid = crypto.randomUUID();

  return `WebTesting-${yyyy}${mm}${dd}-${hh}${min}${ss}UTC-${guid}@OpenAgentFramework.com`;
}

const TEST_PASSWORD = 'TestPassword123!';
const SCREENSHOT_DIR = '../docs/webui/screenshots';

test.describe('WebUI Screenshots', () => {
  test('capture all page screenshots', async ({ page }) => {
    // Set viewport for consistent screenshots
    await page.setViewportSize({ width: 1440, height: 900 });

    // --- Login Page ---
    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/login.png`, fullPage: false });

    // --- Register Page ---
    await page.goto('/register');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Create account' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/register.png`, fullPage: false });

    // --- Register a user to access authenticated pages ---
    const testEmail = generateTestEmail();
    await page.getByLabel('Display name').fill('Screenshot Test User');
    await page.getByLabel('Email').fill(testEmail);
    await page.getByLabel('Password', { exact: true }).fill(TEST_PASSWORD);
    await page.getByLabel('Confirm password').fill(TEST_PASSWORD);
    await page.getByRole('button', { name: 'Create account' }).click();
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10000 });

    // --- Dashboard Page ---
    await page.goto('/dashboard');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    // Wait a moment for charts to render
    await page.waitForTimeout(1000);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/dashboard.png`, fullPage: true });

    // --- Agents Page ---
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Agents' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/agents.png`, fullPage: false });

    // --- Event Feed Page ---
    await page.goto('/events');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Event Feed' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/events.png`, fullPage: false });

    // --- API Keys Page ---
    await page.goto('/settings/api-keys');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'API Keys' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/api-keys.png`, fullPage: false });

    // --- Create an API Key to show the modal ---
    await page.getByPlaceholder('Key name').fill('demo-agent-key');
    await page.getByRole('button', { name: 'Create' }).click();
    // Wait for modal to appear
    await page.waitForTimeout(500);
    const modal = page.locator('.fixed.inset-0');
    if (await modal.isVisible()) {
      await page.screenshot({ path: `${SCREENSHOT_DIR}/api-key-created.png`, fullPage: false });
      // Close the modal
      await page.getByRole('button', { name: 'Done' }).click();
    }

    // --- Organization Settings Page ---
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Organization Settings' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/org-settings.png`, fullPage: true });

    // --- Audit Log Page ---
    await page.goto('/audit');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Audit Log' })).toBeVisible();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/audit-log.png`, fullPage: false });
  });
});

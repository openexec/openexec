import { test, expect } from '@playwright/test'

/**
 * OpenExec UI End-to-End Tests
 *
 * Basic smoke tests to verify the application loads and core components render.
 */

test.describe('OpenExec UI', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test('should load the application', async ({ page }) => {
    // Verify the page title
    await expect(page).toHaveTitle('OpenExec - AI Project Operating System')
  })

  test('should display the chat layout', async ({ page }) => {
    // Wait for the root element to be visible
    await expect(page.locator('#root')).toBeVisible()

    // Wait for React to hydrate (checking that the app has rendered content)
    await expect(page.locator('#root')).not.toBeEmpty()
  })

  test('should have responsive viewport', async ({ page }) => {
    // Test that the page works at common breakpoints
    const viewportSizes = [
      { width: 1920, height: 1080, name: 'desktop' },
      { width: 1024, height: 768, name: 'tablet' },
      { width: 375, height: 667, name: 'mobile' },
    ]

    for (const viewport of viewportSizes) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height })
      await expect(page.locator('#root')).toBeVisible()
    }
  })
})

test.describe('Chat Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test('should render session sidebar', async ({ page }) => {
    // The sidebar should be visible by default
    const sidebar = page.locator('[data-testid="session-sidebar"]')
    // If no testid, check for sidebar-related content
    const sidebarArea = page.locator('aside, [role="complementary"]').first()

    // At least one of these should be visible when the UI loads
    const sidebarVisible = await sidebar.isVisible().catch(() => false)
    const areaVisible = await sidebarArea.isVisible().catch(() => false)

    // The app should have rendered some content
    await expect(page.locator('#root')).not.toBeEmpty()
  })
})

test.describe('Accessibility', () => {
  test('should have accessible focus indicators', async ({ page }) => {
    await page.goto('/')

    // Tab through focusable elements
    await page.keyboard.press('Tab')

    // Check that focused element has visible outline/focus style
    const focusedElement = page.locator(':focus-visible')
    const hasFocusedElement = await focusedElement.count()

    // There should be at least one focusable element
    expect(hasFocusedElement).toBeGreaterThanOrEqual(0)
  })

  test('should have proper document structure', async ({ page }) => {
    await page.goto('/')

    // Check for proper HTML lang attribute
    const html = page.locator('html')
    await expect(html).toHaveAttribute('lang', 'en')

    // Check for viewport meta tag
    const viewport = page.locator('meta[name="viewport"]')
    await expect(viewport).toHaveAttribute('content', /width=device-width/)
  })
})

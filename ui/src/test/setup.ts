/**
 * Vitest Test Setup
 *
 * Configures the test environment with:
 * - jsdom extensions via @testing-library/jest-dom
 * - Mock implementations for browser APIs
 * - Jest compatibility layer for vitest
 *
 * @module test/setup
 */

import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Make vitest available as 'jest' for compatibility with tests written in jest style
// eslint-disable-next-line @typescript-eslint/no-explicit-any
;(globalThis as any).jest = vi

// Mock matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})

// Mock ResizeObserver
class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.ResizeObserver = MockResizeObserver

// Mock IntersectionObserver
class MockIntersectionObserver {
  constructor() {}
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

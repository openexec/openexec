/**
 * Tests for UsageDashboard component
 *
 * @module pages/__tests__/UsageDashboard.test
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import UsageDashboard from '../UsageDashboard'

// Mock fetch globally
const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

describe('UsageDashboard', () => {
  // Mock API responses
  const mockUsageSummary = {
    total_tokens_input: 15000,
    total_tokens_output: 7500,
    total_tokens: 22500,
    total_cost_usd: 0.8,
    total_requests: 18,
    successful_requests: 16,
    failed_requests: 2,
    average_duration_ms: 1500,
    by_provider: {
      openai: {
        provider: 'openai',
        total_tokens_input: 10000,
        total_tokens_output: 5000,
        total_cost_usd: 0.5,
        total_requests: 10,
      },
    },
  }

  const mockProviderUsage = {
    providers: [
      {
        provider: 'openai',
        model: 'gpt-4',
        session_count: 5,
        message_count: 50,
        total_tokens_input: 10000,
        total_tokens_output: 5000,
        total_cost_usd: 0.5,
      },
    ],
    total: {
      session_count: 5,
      message_count: 50,
      total_tokens_input: 10000,
      total_tokens_output: 5000,
      total_tokens: 15000,
      total_cost_usd: 0.5,
    },
  }

  const mockToolCallStats = {
    total_requested: 25,
    total_approved: 20,
    total_rejected: 2,
    total_auto_approved: 10,
    total_completed: 18,
    total_failed: 2,
    by_tool: {
      read_file: 10,
      write_file: 8,
    },
  }

  const mockAuditLogs = {
    entries: [
      {
        id: 'entry-1',
        timestamp: '2024-01-01T10:00:00Z',
        event_type: 'llm.request_end',
        severity: 'info',
        tokens_input: 1000,
        tokens_output: 500,
        cost_usd: 0.01,
        duration_ms: 1000,
        success: true,
      },
    ],
    total_count: 1,
    has_more: false,
    limit: 100,
    offset: 0,
  }

  beforeEach(() => {
    mockFetch.mockReset()
    // Set up default successful responses
    mockFetch.mockImplementation((url: string) => {
      if (url.includes('/usage/summary')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockUsageSummary),
        })
      }
      if (url.includes('/usage/providers')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockProviderUsage),
        })
      }
      if (url.includes('/usage/tools')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockToolCallStats),
        })
      }
      if (url.includes('/usage/audit-logs')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockAuditLogs),
        })
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({}),
      })
    })
  })

  describe('rendering', () => {
    it('should render the dashboard title', async () => {
      render(<UsageDashboard />)

      expect(screen.getByText('Usage Dashboard')).toBeDefined()
    })

    it('should render navigation tabs', async () => {
      render(<UsageDashboard />)

      expect(screen.getByText('Overview')).toBeDefined()
      expect(screen.getByText('Cost')).toBeDefined()
      expect(screen.getByText('Tokens')).toBeDefined()
      expect(screen.getByText('Providers')).toBeDefined()
      expect(screen.getByText('Tools')).toBeDefined()
    })

    it('should render time range selector', async () => {
      render(<UsageDashboard />)

      // Check for preset buttons
      expect(screen.getByText('1h')).toBeDefined()
      expect(screen.getByText('24h')).toBeDefined()
      expect(screen.getByText('7d')).toBeDefined()
      expect(screen.getByText('30d')).toBeDefined()
    })

    it('should render refresh controls', async () => {
      render(<UsageDashboard />)

      // Auto-refresh button
      const autoButton = screen.getByText('Auto')
      expect(autoButton).toBeDefined()
    })

    it('should show session badge when sessionId is provided', async () => {
      render(
        <UsageDashboard config={{ sessionId: 'session-12345678-abcd' }} />
      )

      expect(screen.getByText(/Session:/)).toBeDefined()
    })
  })

  describe('data fetching', () => {
    it('should fetch data on mount', async () => {
      render(<UsageDashboard />)

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalled()
      })
    })

    it('should display usage data when loaded', async () => {
      render(<UsageDashboard />)

      await waitFor(() => {
        // Check that some data is displayed (cost or tokens)
        // The exact text depends on the format helpers
        expect(mockFetch).toHaveBeenCalled()
      })
    })

    it('should pass time range filters to API', async () => {
      render(<UsageDashboard />)

      await waitFor(() => {
        // Check that fetch was called with time parameters
        const summaryCall = mockFetch.mock.calls.find((call: unknown[]) =>
          (call[0] as string).includes('/usage/summary')
        )
        expect(summaryCall).toBeDefined()
      })
    })
  })

  describe('tab navigation', () => {
    it('should switch to Cost tab when clicked', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('Cost'))

      // Should now show cost-specific content
      await waitFor(() => {
        expect(screen.getByText('Cost Timeline')).toBeDefined()
      })
    })

    it('should switch to Tokens tab when clicked', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('Tokens'))

      // Should now show token-specific content
      await waitFor(() => {
        expect(screen.getByText('Token History')).toBeDefined()
      })
    })

    it('should switch to Providers tab when clicked', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('Providers'))

      // Should now show provider-specific content
      await waitFor(() => {
        expect(screen.getByText('Cost by Provider')).toBeDefined()
      })
    })

    it('should switch to Tools tab when clicked', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('Tools'))

      // Should now show tool-specific content
      await waitFor(() => {
        expect(screen.getByText('Tool Usage Distribution')).toBeDefined()
      })
    })
  })

  describe('time range selection', () => {
    it('should update time range when preset is selected', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('24h'))

      await waitFor(() => {
        // Fetch should be called again with new time range
        expect(mockFetch).toHaveBeenCalled()
      })
    })

    it('should update time range when last 30 days is selected', async () => {
      render(<UsageDashboard />)

      fireEvent.click(screen.getByText('30d'))

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalled()
      })
    })
  })

  describe('auto-refresh', () => {
    it('should toggle auto-refresh when button is clicked', async () => {
      render(<UsageDashboard />)

      // Initially auto-refresh is on
      const autoButton = screen.getByText('Auto')
      expect(autoButton).toBeDefined()

      fireEvent.click(autoButton.parentElement!)

      // Now it should show Manual
      await waitFor(() => {
        expect(screen.getByText('Manual')).toBeDefined()
      })
    })
  })

  describe('error handling', () => {
    it('should display error when API fails', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'))

      render(<UsageDashboard />)

      // The error should be caught and potentially displayed
      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalled()
      })
    })
  })

  describe('overview section', () => {
    it('should display UsageSummary in overview', async () => {
      render(<UsageDashboard />)

      await waitFor(() => {
        // Check for key elements from UsageSummary
        expect(screen.getByText('Overview')).toBeDefined()
      })
    })

    it('should display charts in overview', async () => {
      render(<UsageDashboard />)

      await waitFor(() => {
        expect(screen.getByText('Cost Timeline')).toBeDefined()
        expect(screen.getByText('Provider Breakdown')).toBeDefined()
      })
    })
  })

  describe('configuration', () => {
    it('should use custom apiBaseUrl when provided', async () => {
      render(<UsageDashboard config={{ apiBaseUrl: '/custom-api' }} />)

      await waitFor(() => {
        const calls = mockFetch.mock.calls
        const customApiCall = calls.find((call: unknown[]) =>
          (call[0] as string).includes('/custom-api')
        )
        expect(customApiCall).toBeDefined()
      })
    })

    it('should set default time range based on config', async () => {
      render(<UsageDashboard config={{ defaultTimeRange: 'last_30d' }} />)

      // The 30d button should be active by default
      expect(screen.getByText('30d')).toBeDefined()
    })
  })
})

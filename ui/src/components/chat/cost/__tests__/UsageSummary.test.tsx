/**
 * UsageSummary Component Tests
 *
 * Tests for the aggregated usage statistics display component.
 *
 * @module components/chat/cost/__tests__/UsageSummary.test
 */

import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import UsageSummary from '../UsageSummary'
import type { UsageSummaryData } from '../../../../types/chat'

// Helper to create test data
const createTestData = (overrides: Partial<UsageSummaryData> = {}): UsageSummaryData => ({
  usage: {
    totalTokensInput: 50000,
    totalTokensOutput: 25000,
    totalCostUsd: 1.5,
    totalRequests: 100,
    successfulRequests: 95,
    failedRequests: 5,
    averageDurationMs: 1500,
    byProvider: {
      anthropic: {
        provider: 'anthropic',
        totalTokensInput: 30000,
        totalTokensOutput: 15000,
        totalCostUsd: 1.0,
        totalRequests: 60,
      },
      openai: {
        provider: 'openai',
        totalTokensInput: 20000,
        totalTokensOutput: 10000,
        totalCostUsd: 0.5,
        totalRequests: 40,
      },
    },
  },
  toolCalls: {
    totalRequested: 50,
    totalApproved: 20,
    totalRejected: 5,
    totalAutoApproved: 25,
    totalCompleted: 43,
    totalFailed: 2,
    byTool: {
      read_file: 20,
      write_file: 15,
      run_shell_command: 10,
      git_apply_patch: 5,
    },
  },
  activeSessionCount: 3,
  totalSessionCount: 10,
  ...overrides,
})

describe('UsageSummary', () => {
  describe('Basic Rendering', () => {
    it('renders the component title', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Usage Summary')).toBeInTheDocument()
    })

    it('displays total cost', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('$1.50')).toBeInTheDocument()
      expect(screen.getByText('Total Cost')).toBeInTheDocument()
    })

    it('displays total tokens', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('75.0K')).toBeInTheDocument() // 50K + 25K
      expect(screen.getByText('Total Tokens')).toBeInTheDocument()
    })

    it('displays API requests count', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('100')).toBeInTheDocument()
      expect(screen.getByText('API Requests')).toBeInTheDocument()
    })

    it('displays tool calls count', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('43')).toBeInTheDocument() // totalCompleted
      // Tool Calls appears both in the card and section title, use getAllByText
      expect(screen.getAllByText('Tool Calls').length).toBeGreaterThan(0)
    })
  })

  describe('Variant Display', () => {
    it('hides detailed sections in compact mode', () => {
      const data = createTestData()
      render(<UsageSummary data={data} variant="compact" />)

      // Main stats should still be visible
      expect(screen.getByText('Total Cost')).toBeInTheDocument()

      // Detailed sections should be hidden
      expect(screen.queryByText('Token Breakdown')).not.toBeInTheDocument()
      expect(screen.queryByText('Request Statistics')).not.toBeInTheDocument()
      expect(screen.queryByText('By Provider')).not.toBeInTheDocument()
    })

    it('shows all sections in detailed mode', () => {
      const data = createTestData()
      render(<UsageSummary data={data} variant="detailed" />)

      expect(screen.getByText('Token Breakdown')).toBeInTheDocument()
      expect(screen.getByText('Request Statistics')).toBeInTheDocument()
      expect(screen.getByText('By Provider')).toBeInTheDocument()
      // Tool Calls appears both as section title and card label
      expect(screen.getAllByText('Tool Calls').length).toBe(2)
    })
  })

  describe('Token Breakdown', () => {
    it('displays input and output token counts', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Input Tokens')).toBeInTheDocument()
      expect(screen.getByText('50.0K')).toBeInTheDocument()
      expect(screen.getByText('Output Tokens')).toBeInTheDocument()
      expect(screen.getByText('25.0K')).toBeInTheDocument()
    })
  })

  describe('Request Statistics', () => {
    it('displays successful and failed request counts', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Successful')).toBeInTheDocument()
      expect(screen.getByText('95')).toBeInTheDocument()
      expect(screen.getByText('Failed')).toBeInTheDocument()
      // '5' appears twice (failed requests and rejected tool calls), use getAllByText
      expect(screen.getAllByText('5').length).toBeGreaterThan(0)
    })

    it('displays success rate', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Success Rate')).toBeInTheDocument()
      expect(screen.getByText('95.0%')).toBeInTheDocument()
    })

    it('displays average duration', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Avg Duration')).toBeInTheDocument()
      expect(screen.getByText('1.5s')).toBeInTheDocument()
    })
  })

  describe('Provider Breakdown', () => {
    it('displays provider names', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('anthropic')).toBeInTheDocument()
      expect(screen.getByText('openai')).toBeInTheDocument()
    })

    it('displays provider request counts', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('60 requests')).toBeInTheDocument()
      expect(screen.getByText('40 requests')).toBeInTheDocument()
    })

    it('hides provider breakdown when showProviderBreakdown is false', () => {
      const data = createTestData()
      render(<UsageSummary data={data} showProviderBreakdown={false} />)

      expect(screen.queryByText('By Provider')).not.toBeInTheDocument()
    })
  })

  describe('Tool Call Statistics', () => {
    it('displays tool call metrics', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Requested')).toBeInTheDocument()
      expect(screen.getByText('50')).toBeInTheDocument()
      expect(screen.getByText('Approved')).toBeInTheDocument()
      expect(screen.getByText('20')).toBeInTheDocument()
      expect(screen.getByText('Auto-approved')).toBeInTheDocument()
      expect(screen.getByText('25')).toBeInTheDocument()
      expect(screen.getByText('Rejected')).toBeInTheDocument()
    })

    it('displays top tools', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('Top Tools:')).toBeInTheDocument()
      expect(screen.getByText(/read_file/)).toBeInTheDocument()
      expect(screen.getByText(/write_file/)).toBeInTheDocument()
    })

    it('hides tool stats when showToolStats is false', () => {
      const data = createTestData()
      render(<UsageSummary data={data} showToolStats={false} />)

      // Tool Calls should not appear in section or card
      expect(screen.queryAllByText('Tool Calls').length).toBe(0)
      expect(screen.queryByText('Top Tools:')).not.toBeInTheDocument()
    })

    it('hides tool call card when showToolStats is false', () => {
      const data = createTestData()
      render(<UsageSummary data={data} showToolStats={false} />)

      // Should have 3 cards instead of 4 in the main grid
      const cardLabels = ['Total Cost', 'Total Tokens', 'API Requests']
      cardLabels.forEach((label) => {
        expect(screen.getByText(label)).toBeInTheDocument()
      })
      // Tool Calls label should not appear anywhere
      expect(screen.queryAllByText('Tool Calls').length).toBe(0)
    })
  })

  describe('Period Display', () => {
    it('displays All time when no period is specified', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('All time')).toBeInTheDocument()
    })

    it('displays formatted date range when period is specified', () => {
      const data = createTestData({
        periodStart: '2024-01-01T00:00:00Z',
        periodEnd: '2024-01-31T23:59:59Z',
      })
      render(<UsageSummary data={data} />)

      // Date format may vary based on locale, just check that All time is not shown
      expect(screen.queryByText('All time')).not.toBeInTheDocument()
      // Check that we have a period span element with date content
      const periodElement = screen.getByText(/Jan/)
      expect(periodElement).toBeInTheDocument()
    })

    it('hides period when showPeriod is false', () => {
      const data = createTestData({
        periodStart: '2024-01-01T00:00:00Z',
        periodEnd: '2024-01-31T23:59:59Z',
      })
      render(<UsageSummary data={data} showPeriod={false} />)

      // No period indicator should be shown (All time or date range)
      expect(screen.queryByText('All time')).not.toBeInTheDocument()
      // The period span element should not exist
      const periodSpan = document.querySelector('.usage-summary__period')
      expect(periodSpan).not.toBeInTheDocument()
    })
  })

  describe('Session Counts', () => {
    it('displays active session count', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('3')).toBeInTheDocument()
      expect(screen.getByText(/active sessions/)).toBeInTheDocument()
    })

    it('displays total session count', () => {
      const data = createTestData()
      render(<UsageSummary data={data} />)

      expect(screen.getByText('10')).toBeInTheDocument()
      expect(screen.getByText(/total sessions/)).toBeInTheDocument()
    })
  })

  describe('Edge Cases', () => {
    it('handles zero values gracefully', () => {
      const data = createTestData({
        usage: {
          totalTokensInput: 0,
          totalTokensOutput: 0,
          totalCostUsd: 0,
          totalRequests: 0,
          successfulRequests: 0,
          failedRequests: 0,
          averageDurationMs: 0,
        },
        toolCalls: {
          totalRequested: 0,
          totalApproved: 0,
          totalRejected: 0,
          totalAutoApproved: 0,
          totalCompleted: 0,
          totalFailed: 0,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('$0.00')).toBeInTheDocument()
      // Multiple elements will have '0' value, just check at least one exists
      expect(screen.getAllByText('0').length).toBeGreaterThan(0)
    })

    it('handles very small costs', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          totalCostUsd: 0.00001,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('<$0.0001')).toBeInTheDocument()
    })

    it('handles large token counts with M suffix', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          totalTokensInput: 5000000,
          totalTokensOutput: 2500000,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('7.5M')).toBeInTheDocument() // 5M + 2.5M total
    })

    it('handles missing provider breakdown', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          byProvider: undefined,
        },
      })
      render(<UsageSummary data={data} />)

      // Should not show provider section when no providers
      expect(screen.queryByText('By Provider')).not.toBeInTheDocument()
    })

    it('handles missing tool breakdown', () => {
      const data = createTestData({
        toolCalls: {
          ...createTestData().toolCalls,
          byTool: undefined,
        },
      })
      render(<UsageSummary data={data} />)

      // Should not show top tools section
      expect(screen.queryByText('Top Tools:')).not.toBeInTheDocument()
    })

    it('handles missing session counts', () => {
      const data = createTestData({
        activeSessionCount: undefined,
        totalSessionCount: undefined,
      })
      render(<UsageSummary data={data} />)

      expect(screen.queryByText(/active sessions/)).not.toBeInTheDocument()
      expect(screen.queryByText(/total sessions/)).not.toBeInTheDocument()
    })
  })

  describe('Duration Formatting', () => {
    it('formats milliseconds correctly', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          averageDurationMs: 500,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('500ms')).toBeInTheDocument()
    })

    it('formats seconds correctly', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          averageDurationMs: 2500,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('2.5s')).toBeInTheDocument()
    })

    it('formats minutes correctly', () => {
      const data = createTestData({
        usage: {
          ...createTestData().usage,
          averageDurationMs: 120000,
        },
      })
      render(<UsageSummary data={data} />)

      expect(screen.getByText('2.0m')).toBeInTheDocument()
    })
  })
})

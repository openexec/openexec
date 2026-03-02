/**
 * UsageChartPanel Component Tests
 *
 * @module components/chat/cost/__tests__/UsageChartPanel.test
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import UsageChartPanel from '../UsageChartPanel'
import type { UsageStats, ProviderStats, ToolCallStats } from '../../../../types/chat'
import type { TokenHistoryDataPoint } from '../TokenHistoryChart'

describe('UsageChartPanel', () => {
  const mockUsage: UsageStats = {
    totalTokensInput: 15000,
    totalTokensOutput: 7500,
    totalCostUsd: 0.80,
    totalRequests: 18,
    successfulRequests: 16,
    failedRequests: 2,
    averageDurationMs: 1500,
  }

  const mockProviders: ProviderStats[] = [
    {
      provider: 'openai',
      totalTokensInput: 10000,
      totalTokensOutput: 5000,
      totalCostUsd: 0.50,
      totalRequests: 10,
    },
    {
      provider: 'anthropic',
      totalTokensInput: 5000,
      totalTokensOutput: 2500,
      totalCostUsd: 0.30,
      totalRequests: 8,
    },
  ]

  const mockToolCalls: ToolCallStats = {
    totalRequested: 25,
    totalApproved: 20,
    totalRejected: 2,
    totalAutoApproved: 10,
    totalCompleted: 18,
    totalFailed: 2,
    byTool: {
      'read_file': 10,
      'write_file': 8,
      'run_shell_command': 7,
    },
  }

  const mockTokenHistory: TokenHistoryDataPoint[] = [
    { timestamp: '2024-01-01T10:00:00Z', inputTokens: 1000, outputTokens: 500 },
    { timestamp: '2024-01-01T10:05:00Z', inputTokens: 2000, outputTokens: 1000 },
    { timestamp: '2024-01-01T10:10:00Z', inputTokens: 3000, outputTokens: 1500 },
  ]

  describe('header', () => {
    it('renders panel title', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('Usage Analytics')).toBeInTheDocument()
    })

    it('shows loading indicator when loading', () => {
      render(<UsageChartPanel usage={mockUsage} loading={true} />)
      expect(screen.getByText('Loading...')).toBeInTheDocument()
    })

    it('shows refresh button when onRefresh is provided', () => {
      const onRefresh = vi.fn()
      render(<UsageChartPanel usage={mockUsage} onRefresh={onRefresh} />)
      const button = screen.getByTitle('Refresh data')
      expect(button).toBeInTheDocument()
    })

    it('calls onRefresh when refresh button is clicked', () => {
      const onRefresh = vi.fn()
      render(<UsageChartPanel usage={mockUsage} onRefresh={onRefresh} />)
      const button = screen.getByTitle('Refresh data')
      fireEvent.click(button)
      expect(onRefresh).toHaveBeenCalledTimes(1)
    })
  })

  describe('collapse/expand', () => {
    it('shows collapsed summary when collapsed', () => {
      const { container } = render(<UsageChartPanel usage={mockUsage} />)

      // Find and click collapse button
      const collapseButton = container.querySelector('button[aria-label="Collapse"]')
      expect(collapseButton).toBeInTheDocument()
      fireEvent.click(collapseButton!)

      // Should show collapsed summary
      const collapsedSummary = container.querySelector('.usage-chart-panel__tabs')
      expect(collapsedSummary).not.toBeInTheDocument()
    })

    it('expands when expand button is clicked', () => {
      const { container } = render(<UsageChartPanel usage={mockUsage} />)

      // Collapse first
      const collapseButton = container.querySelector('button[aria-label="Collapse"]')
      fireEvent.click(collapseButton!)

      // Then expand
      const expandButton = container.querySelector('button[aria-label="Expand"]')
      fireEvent.click(expandButton!)

      // Tabs should be visible again
      const tabs = container.querySelector('.usage-chart-panel__tabs')
      expect(tabs).toBeInTheDocument()
    })
  })

  describe('tabs', () => {
    it('renders all tab buttons', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('Overview')).toBeInTheDocument()
      expect(screen.getByText('Providers')).toBeInTheDocument()
      expect(screen.getByText('Tokens')).toBeInTheDocument()
      expect(screen.getByText('Tools')).toBeInTheDocument()
    })

    it('switches to Providers tab on click', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          providers={mockProviders}
        />
      )
      fireEvent.click(screen.getByText('Providers'))
      // Provider chart should be visible
      expect(screen.getByText('Usage by Provider')).toBeInTheDocument()
    })

    it('switches to Tokens tab on click', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          tokenHistory={mockTokenHistory}
        />
      )
      fireEvent.click(screen.getByText('Tokens'))
      expect(screen.getByText('Token Usage Over Time')).toBeInTheDocument()
    })

    it('switches to Tools tab on click', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          toolCalls={mockToolCalls}
        />
      )
      fireEvent.click(screen.getByText('Tools'))
      expect(screen.getByText('Total Requested')).toBeInTheDocument()
    })
  })

  describe('overview section', () => {
    it('shows key metrics', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('Total Cost')).toBeInTheDocument()
      expect(screen.getByText('Total Tokens')).toBeInTheDocument()
      expect(screen.getByText('Requests')).toBeInTheDocument()
      expect(screen.getByText('Avg Duration')).toBeInTheDocument()
    })

    it('displays formatted cost', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('$0.80')).toBeInTheDocument()
    })

    it('displays total token count', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      // 15000 + 7500 = 22500 -> 22.5K
      expect(screen.getByText('22.5K')).toBeInTheDocument()
    })

    it('displays request count', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('18')).toBeInTheDocument()
    })

    it('displays average duration', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      expect(screen.getByText('1500ms')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('displays error message', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          error="Failed to load usage data"
        />
      )
      expect(screen.getByText('Failed to load usage data')).toBeInTheDocument()
    })
  })

  describe('empty states', () => {
    it('shows empty message when no usage data', () => {
      render(<UsageChartPanel />)
      expect(screen.getByText('No usage data available')).toBeInTheDocument()
    })

    it('shows empty message on tools tab with no tool data', () => {
      render(<UsageChartPanel usage={mockUsage} />)
      fireEvent.click(screen.getByText('Tools'))
      expect(screen.getByText('No tool call data available')).toBeInTheDocument()
    })
  })

  describe('auto-refresh', () => {
    beforeEach(() => {
      vi.useFakeTimers()
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('calls onRefresh at interval when autoRefresh is true', () => {
      const onRefresh = vi.fn()
      render(
        <UsageChartPanel
          usage={mockUsage}
          autoRefresh={true}
          refreshInterval={1000}
          onRefresh={onRefresh}
        />
      )

      // Should not be called immediately
      expect(onRefresh).not.toHaveBeenCalled()

      // Advance timer
      vi.advanceTimersByTime(1000)
      expect(onRefresh).toHaveBeenCalledTimes(1)

      // Advance again
      vi.advanceTimersByTime(1000)
      expect(onRefresh).toHaveBeenCalledTimes(2)
    })

    it('does not call onRefresh when autoRefresh is false', () => {
      const onRefresh = vi.fn()
      render(
        <UsageChartPanel
          usage={mockUsage}
          autoRefresh={false}
          refreshInterval={1000}
          onRefresh={onRefresh}
        />
      )

      vi.advanceTimersByTime(5000)
      expect(onRefresh).not.toHaveBeenCalled()
    })
  })

  describe('tools view', () => {
    it('displays tool call summary stats', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          toolCalls={mockToolCalls}
        />
      )
      fireEvent.click(screen.getByText('Tools'))

      expect(screen.getByText('25')).toBeInTheDocument() // Total requested
      expect(screen.getByText('Approved')).toBeInTheDocument()
      expect(screen.getByText('Rejected')).toBeInTheDocument()
      expect(screen.getByText('Auto-Approved')).toBeInTheDocument()
      expect(screen.getByText('Completed')).toBeInTheDocument()
      expect(screen.getByText('Failed')).toBeInTheDocument()
    })

    it('displays tool breakdown chart', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          toolCalls={mockToolCalls}
        />
      )
      fireEvent.click(screen.getByText('Tools'))

      expect(screen.getByText('Calls by Tool')).toBeInTheDocument()
      expect(screen.getByText('read_file')).toBeInTheDocument()
      expect(screen.getByText('write_file')).toBeInTheDocument()
      expect(screen.getByText('run_shell_command')).toBeInTheDocument()
    })
  })

  describe('providers view', () => {
    it('displays provider usage chart', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          providers={mockProviders}
        />
      )
      fireEvent.click(screen.getByText('Providers'))

      expect(screen.getByText('Usage by Provider')).toBeInTheDocument()
      expect(screen.getByText('openai')).toBeInTheDocument()
      expect(screen.getByText('anthropic')).toBeInTheDocument()
    })
  })

  describe('tokens view', () => {
    it('displays token history chart', () => {
      render(
        <UsageChartPanel
          usage={mockUsage}
          tokenHistory={mockTokenHistory}
        />
      )
      fireEvent.click(screen.getByText('Tokens'))

      expect(screen.getByText('Token Usage Over Time')).toBeInTheDocument()
    })
  })
})

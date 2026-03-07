/**
 * UsageChartPanel Component Tests - Minimal Version
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import UsageChartPanel from '../UsageChartPanel'

// Mock everything heavy or async
vi.mock('../UsageBarChart', () => ({
  default: () => <div data-testid="mock-usage-bar-chart" />,
  formatCost: (v: number) => `$${v}`,
  formatNumber: (v: number) => `${v}`,
}))

vi.mock('../ProviderUsageChart', () => ({
  default: () => <div data-testid="mock-provider-usage-chart" />
}))

vi.mock('../TokenHistoryChart', () => ({
  default: () => <div data-testid="mock-token-history-chart" />
}))

describe('UsageChartPanel Minimal', () => {
  const mockUsage = {
    totalTokensInput: 1000,
    totalTokensOutput: 500,
    totalCostUsd: 0.10,
    totalRequests: 5,
    successfulRequests: 5,
    failedRequests: 0,
    averageDurationMs: 1000,
  }

  it('renders without crashing', () => {
    render(<UsageChartPanel usage={mockUsage} />)
    expect(screen.getByText('Usage Analytics')).toBeInTheDocument()
  })
})

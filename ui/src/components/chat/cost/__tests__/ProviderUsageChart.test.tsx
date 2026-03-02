/**
 * ProviderUsageChart Component Tests
 *
 * @module components/chat/cost/__tests__/ProviderUsageChart.test
 */

import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import ProviderUsageChart from '../ProviderUsageChart'
import type { ProviderStats } from '../../../../types/chat'

describe('ProviderUsageChart', () => {
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
      totalTokensInput: 8000,
      totalTokensOutput: 4000,
      totalCostUsd: 0.30,
      totalRequests: 8,
    },
  ]

  describe('rendering', () => {
    it('renders with title', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      expect(screen.getByText('Usage by Provider')).toBeInTheDocument()
    })

    it('renders custom title', () => {
      render(
        <ProviderUsageChart
          providers={mockProviders}
          title="Custom Title"
        />
      )
      expect(screen.getByText('Custom Title')).toBeInTheDocument()
    })

    it('renders provider names in legend', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      expect(screen.getByText('openai')).toBeInTheDocument()
      expect(screen.getByText('anthropic')).toBeInTheDocument()
    })

    it('renders total cost in center', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      // Total cost is 0.50 + 0.30 = 0.80
      expect(screen.getByText('$0.80')).toBeInTheDocument()
    })

    it('renders Total label', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      expect(screen.getByText('Total')).toBeInTheDocument()
    })

    it('renders summary statistics', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      expect(screen.getByText('Total Tokens')).toBeInTheDocument()
      expect(screen.getByText('Providers')).toBeInTheDocument()
      expect(screen.getByText('Requests')).toBeInTheDocument()
    })

    it('shows correct provider count', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      // Provider count should be 2
      expect(screen.getByText('2')).toBeInTheDocument()
    })

    it('shows correct total requests', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      // Total requests: 10 + 8 = 18
      expect(screen.getByText('18')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('renders empty message when no providers', () => {
      render(
        <ProviderUsageChart
          providers={[]}
          emptyMessage="No usage data"
        />
      )
      expect(screen.getByText('No usage data')).toBeInTheDocument()
    })

    it('renders empty state when total cost is zero', () => {
      const zeroCostProviders: ProviderStats[] = [
        {
          provider: 'test',
          totalTokensInput: 0,
          totalTokensOutput: 0,
          totalCostUsd: 0,
          totalRequests: 0,
        },
      ]
      render(<ProviderUsageChart providers={zeroCostProviders} />)
      expect(screen.getByText('No usage data')).toBeInTheDocument()
    })
  })

  describe('details display', () => {
    it('shows details when showDetails is true', () => {
      render(
        <ProviderUsageChart
          providers={mockProviders}
          showDetails={true}
        />
      )
      // Should show cost details like "$0.50"
      expect(screen.getByText('$0.50')).toBeInTheDocument()
      // Should show request count
      expect(screen.getByText('10 requests')).toBeInTheDocument()
    })

    it('hides details when showDetails is false', () => {
      render(
        <ProviderUsageChart
          providers={mockProviders}
          showDetails={false}
        />
      )
      // Should not show individual cost details (only total)
      expect(screen.queryByText('10 requests')).not.toBeInTheDocument()
    })
  })

  describe('percentages', () => {
    it('calculates correct percentages', () => {
      render(<ProviderUsageChart providers={mockProviders} />)
      // OpenAI: 0.50 / 0.80 = 62.5%
      expect(screen.getByText('62.5%')).toBeInTheDocument()
      // Anthropic: 0.30 / 0.80 = 37.5%
      expect(screen.getByText('37.5%')).toBeInTheDocument()
    })
  })

  describe('custom totalCost', () => {
    it('uses provided totalCost for center display', () => {
      render(
        <ProviderUsageChart
          providers={mockProviders}
          totalCost={1.00}
        />
      )
      expect(screen.getByText('$1.00')).toBeInTheDocument()
    })
  })

  describe('SVG rendering', () => {
    it('renders SVG element', () => {
      const { container } = render(
        <ProviderUsageChart providers={mockProviders} />
      )
      const svg = container.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('renders correct number of segment circles', () => {
      const { container } = render(
        <ProviderUsageChart providers={mockProviders} />
      )
      // Should have 1 background circle + 2 segment circles
      const circles = container.querySelectorAll('circle')
      expect(circles.length).toBe(3)
    })
  })

  describe('size prop', () => {
    it('respects custom size', () => {
      const { container } = render(
        <ProviderUsageChart providers={mockProviders} size={200} />
      )
      const svg = container.querySelector('svg')
      expect(svg).toHaveAttribute('width', '200')
      expect(svg).toHaveAttribute('height', '200')
    })
  })
})

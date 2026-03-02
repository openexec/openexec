/**
 * UsageBarChart Component Tests
 *
 * @module components/chat/cost/__tests__/UsageBarChart.test
 */

import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import UsageBarChart, { formatNumber, formatCost } from '../UsageBarChart'

describe('UsageBarChart', () => {
  const mockData = [
    { label: 'OpenAI', value: 1500, sublabel: 'gpt-4' },
    { label: 'Anthropic', value: 800, sublabel: 'claude-3' },
    { label: 'Google', value: 300, sublabel: 'gemini-pro' },
  ]

  describe('rendering', () => {
    it('renders with title', () => {
      render(<UsageBarChart data={mockData} title="Cost Breakdown" />)
      expect(screen.getByText('Cost Breakdown')).toBeInTheDocument()
    })

    it('renders all data labels', () => {
      render(<UsageBarChart data={mockData} />)
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
      expect(screen.getByText('Anthropic')).toBeInTheDocument()
      expect(screen.getByText('Google')).toBeInTheDocument()
    })

    it('renders sublabels when provided', () => {
      render(<UsageBarChart data={mockData} />)
      expect(screen.getByText('gpt-4')).toBeInTheDocument()
      expect(screen.getByText('claude-3')).toBeInTheDocument()
      expect(screen.getByText('gemini-pro')).toBeInTheDocument()
    })

    it('renders empty state when no data', () => {
      render(<UsageBarChart data={[]} emptyMessage="No data available" />)
      expect(screen.getByText('No data available')).toBeInTheDocument()
    })

    it('renders total row when multiple items', () => {
      render(<UsageBarChart data={mockData} />)
      expect(screen.getByText('Total')).toBeInTheDocument()
    })

    it('does not render total row for single item', () => {
      render(<UsageBarChart data={[mockData[0]]} />)
      expect(screen.queryByText('Total')).not.toBeInTheDocument()
    })
  })

  describe('percentages', () => {
    it('shows percentages when showPercentage is true', () => {
      render(<UsageBarChart data={mockData} showPercentage={true} />)
      // Total is 2600, OpenAI is 1500 = 57.7%
      expect(screen.getByText('57.7%')).toBeInTheDocument()
    })

    it('hides percentages when showPercentage is false', () => {
      render(<UsageBarChart data={mockData} showPercentage={false} />)
      expect(screen.queryByText('57.7%')).not.toBeInTheDocument()
    })
  })

  describe('custom formatting', () => {
    it('uses custom formatValue function', () => {
      const customFormat = (value: number) => `$${value.toFixed(2)}`
      render(
        <UsageBarChart
          data={[{ label: 'Test', value: 10.5 }]}
          formatValue={customFormat}
        />
      )
      expect(screen.getByText('$10.50')).toBeInTheDocument()
    })
  })

  describe('custom colors', () => {
    it('renders with custom color on data point', () => {
      const dataWithColor = [
        { label: 'Custom', value: 100, color: '#ff0000' },
      ]
      const { container } = render(<UsageBarChart data={dataWithColor} />)
      const fill = container.querySelector('.usage-bar-chart__bar-fill')
      expect(fill).toHaveStyle({ backgroundColor: '#ff0000' })
    })
  })
})

describe('formatNumber', () => {
  it('formats numbers under 1000', () => {
    expect(formatNumber(0)).toBe('0')
    expect(formatNumber(100)).toBe('100')
    expect(formatNumber(999)).toBe('999')
  })

  it('formats thousands with K suffix', () => {
    expect(formatNumber(1000)).toBe('1.0K')
    expect(formatNumber(1500)).toBe('1.5K')
    expect(formatNumber(10000)).toBe('10.0K')
    expect(formatNumber(999999)).toBe('1000.0K')
  })

  it('formats millions with M suffix', () => {
    expect(formatNumber(1000000)).toBe('1.0M')
    expect(formatNumber(1500000)).toBe('1.5M')
    expect(formatNumber(10000000)).toBe('10.0M')
  })
})

describe('formatCost', () => {
  it('formats zero cost', () => {
    expect(formatCost(0)).toBe('$0.00')
  })

  it('formats very small costs', () => {
    expect(formatCost(0.00001)).toBe('<$0.0001')
    expect(formatCost(0.00005)).toBe('<$0.0001')
  })

  it('formats small costs with 4 decimal places', () => {
    expect(formatCost(0.0001)).toBe('$0.0001')
    expect(formatCost(0.001)).toBe('$0.0010')
    expect(formatCost(0.0099)).toBe('$0.0099')
  })

  it('formats normal costs with 2 decimal places', () => {
    expect(formatCost(0.01)).toBe('$0.01')
    expect(formatCost(0.50)).toBe('$0.50')
    expect(formatCost(1.00)).toBe('$1.00')
    expect(formatCost(10.99)).toBe('$10.99')
    expect(formatCost(100.00)).toBe('$100.00')
  })
})

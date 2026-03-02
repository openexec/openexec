/**
 * TokenHistoryChart Component Tests
 *
 * @module components/chat/cost/__tests__/TokenHistoryChart.test
 */

import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import TokenHistoryChart, { type TokenHistoryDataPoint } from '../TokenHistoryChart'

describe('TokenHistoryChart', () => {
  const mockData: TokenHistoryDataPoint[] = [
    {
      timestamp: '2024-01-01T10:00:00Z',
      inputTokens: 1000,
      outputTokens: 500,
      costUsd: 0.01,
      label: 'Iter 1',
    },
    {
      timestamp: '2024-01-01T10:05:00Z',
      inputTokens: 2000,
      outputTokens: 1000,
      costUsd: 0.02,
      label: 'Iter 2',
    },
    {
      timestamp: '2024-01-01T10:10:00Z',
      inputTokens: 3000,
      outputTokens: 1500,
      costUsd: 0.03,
      label: 'Iter 3',
    },
  ]

  describe('rendering', () => {
    it('renders with title', () => {
      render(<TokenHistoryChart data={mockData} />)
      expect(screen.getByText('Token Usage Over Time')).toBeInTheDocument()
    })

    it('renders custom title', () => {
      render(
        <TokenHistoryChart data={mockData} title="Custom Chart Title" />
      )
      expect(screen.getByText('Custom Chart Title')).toBeInTheDocument()
    })

    it('renders legend items', () => {
      render(<TokenHistoryChart data={mockData} />)
      expect(screen.getByText('Input')).toBeInTheDocument()
      expect(screen.getByText('Output')).toBeInTheDocument()
    })

    it('renders cost legend when showCost is true', () => {
      render(<TokenHistoryChart data={mockData} showCost={true} />)
      expect(screen.getByText('Cost')).toBeInTheDocument()
    })

    it('does not render cost legend when showCost is false', () => {
      render(<TokenHistoryChart data={mockData} showCost={false} />)
      expect(screen.queryByText('Cost')).not.toBeInTheDocument()
    })

    it('renders summary statistics', () => {
      render(<TokenHistoryChart data={mockData} />)
      expect(screen.getByText('Current Input')).toBeInTheDocument()
      expect(screen.getByText('Current Output')).toBeInTheDocument()
      expect(screen.getByText('Data Points')).toBeInTheDocument()
    })

    it('shows correct data point count', () => {
      render(<TokenHistoryChart data={mockData} />)
      expect(screen.getByText('3')).toBeInTheDocument()
    })

    it('shows current token values from last data point', () => {
      render(<TokenHistoryChart data={mockData} />)
      // Last data point: 3000 input, 1500 output -> 3.0K, 1.5K
      // Use getAllByText since values appear in both Y-axis and summary
      const threeKElements = screen.getAllByText('3.0K')
      const onePointFiveKElements = screen.getAllByText('1.5K')
      expect(threeKElements.length).toBeGreaterThan(0)
      expect(onePointFiveKElements.length).toBeGreaterThan(0)
    })
  })

  describe('empty state', () => {
    it('renders empty message when no data', () => {
      render(
        <TokenHistoryChart data={[]} emptyMessage="No token history data" />
      )
      expect(screen.getByText('No token history data')).toBeInTheDocument()
    })

    it('does not render SVG when no data', () => {
      const { container } = render(<TokenHistoryChart data={[]} />)
      const svg = container.querySelector('.token-history-chart__svg')
      expect(svg).not.toBeInTheDocument()
    })
  })

  describe('SVG rendering', () => {
    it('renders SVG element', () => {
      const { container } = render(<TokenHistoryChart data={mockData} />)
      const svg = container.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('renders path elements for series', () => {
      const { container } = render(<TokenHistoryChart data={mockData} />)
      const paths = container.querySelectorAll('path')
      // At least 2 line paths (input and output)
      expect(paths.length).toBeGreaterThanOrEqual(2)
    })

    it('renders area fills when showFill is true', () => {
      const { container } = render(
        <TokenHistoryChart data={mockData} showFill={true} />
      )
      // Should have paths with fillOpacity for area fills
      const paths = container.querySelectorAll('path[fill-opacity]')
      expect(paths.length).toBeGreaterThanOrEqual(2)
    })
  })

  describe('X-axis labels', () => {
    it('renders labels for small datasets', () => {
      render(<TokenHistoryChart data={mockData} />)
      // Should show all labels for small dataset
      expect(screen.getByText('Iter 1')).toBeInTheDocument()
      expect(screen.getByText('Iter 2')).toBeInTheDocument()
      expect(screen.getByText('Iter 3')).toBeInTheDocument()
    })
  })

  describe('rolling window (maxPoints)', () => {
    it('limits displayed data to maxPoints', () => {
      const manyDataPoints: TokenHistoryDataPoint[] = Array.from(
        { length: 100 },
        (_, i) => ({
          timestamp: `2024-01-01T10:${i.toString().padStart(2, '0')}:00Z`,
          inputTokens: 1000 + i * 100,
          outputTokens: 500 + i * 50,
          label: `Iter ${i + 1}`,
        })
      )

      render(<TokenHistoryChart data={manyDataPoints} maxPoints={50} />)
      // Should show 50 as data point count
      expect(screen.getByText('50')).toBeInTheDocument()
    })
  })

  describe('interaction', () => {
    it('shows tooltip on hover', () => {
      const { container } = render(<TokenHistoryChart data={mockData} />)

      // Find interactive rect elements
      const rects = container.querySelectorAll('rect[fill="transparent"]')
      expect(rects.length).toBe(3)

      // Hover over first rect
      fireEvent.mouseEnter(rects[0])

      // Tooltip should appear with data point info
      const tooltip = container.querySelector('.token-history-chart__tooltip')
      expect(tooltip).toBeInTheDocument()
    })

    it('hides tooltip on mouse leave', () => {
      const { container } = render(<TokenHistoryChart data={mockData} />)

      const rects = container.querySelectorAll('rect[fill="transparent"]')

      // Hover and then leave
      fireEvent.mouseEnter(rects[0])
      fireEvent.mouseLeave(rects[0])

      // Tooltip should be gone
      const tooltip = container.querySelector('.token-history-chart__tooltip')
      expect(tooltip).not.toBeInTheDocument()
    })
  })

  describe('height prop', () => {
    it('respects custom height', () => {
      const { container } = render(
        <TokenHistoryChart data={mockData} height={200} />
      )
      const svg = container.querySelector('svg')
      expect(svg).toHaveAttribute('height', '200')
    })
  })

  describe('single data point', () => {
    it('renders single data point correctly', () => {
      const singlePoint: TokenHistoryDataPoint[] = [
        {
          timestamp: '2024-01-01T10:00:00Z',
          inputTokens: 1000,
          outputTokens: 500,
          label: 'Only point',
        },
      ]

      render(<TokenHistoryChart data={singlePoint} />)
      expect(screen.getByText('1')).toBeInTheDocument()
      expect(screen.getByText('Only point')).toBeInTheDocument()
    })
  })
})

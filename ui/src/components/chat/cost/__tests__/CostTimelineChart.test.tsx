/**
 * CostTimelineChart Component Tests
 *
 * @module components/chat/cost/__tests__/CostTimelineChart.test
 */

import React from 'react'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import CostTimelineChart, { type CostDataPoint } from '../CostTimelineChart'

describe('CostTimelineChart', () => {
  const mockData: CostDataPoint[] = [
    {
      iteration: 1,
      timestamp: '2024-01-01T10:00:00Z',
      cumulativeCost: 0.01,
      iterationCost: 0.01,
      inputTokens: 1000,
      outputTokens: 500,
    },
    {
      iteration: 2,
      timestamp: '2024-01-01T10:05:00Z',
      cumulativeCost: 0.03,
      iterationCost: 0.02,
      inputTokens: 2000,
      outputTokens: 1000,
    },
    {
      iteration: 3,
      timestamp: '2024-01-01T10:10:00Z',
      cumulativeCost: 0.06,
      iterationCost: 0.03,
      inputTokens: 3000,
      outputTokens: 1500,
    },
  ]

  describe('rendering', () => {
    it('renders with default title', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.getByText('Cost Over Time')).toBeInTheDocument()
    })

    it('renders custom title', () => {
      render(
        <CostTimelineChart data={mockData} title="Session Cost Timeline" />
      )
      expect(screen.getByText('Session Cost Timeline')).toBeInTheDocument()
    })

    it('renders legend items', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.getByText('Cumulative Cost')).toBeInTheDocument()
      expect(screen.getByText('Per Iteration')).toBeInTheDocument()
    })

    it('renders budget legend when budgetLimit is provided', () => {
      render(<CostTimelineChart data={mockData} budgetLimit={1.0} />)
      expect(screen.getByText('Budget')).toBeInTheDocument()
    })

    it('does not render budget legend when no budgetLimit', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.queryByText('Budget')).not.toBeInTheDocument()
    })

    it('hides iteration bars when showIterationBars is false', () => {
      render(<CostTimelineChart data={mockData} showIterationBars={false} />)
      expect(screen.queryByText('Per Iteration')).not.toBeInTheDocument()
    })
  })

  describe('summary statistics', () => {
    it('renders summary section', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.getByText('Current Cost')).toBeInTheDocument()
      expect(screen.getByText('Iterations')).toBeInTheDocument()
      expect(screen.getByText('Avg/Iteration')).toBeInTheDocument()
    })

    it('shows correct iteration count', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.getByText('3')).toBeInTheDocument()
    })

    it('displays current cost from last data point', () => {
      render(<CostTimelineChart data={mockData} />)
      // Last cumulative cost is $0.06
      expect(screen.getByText('$0.060')).toBeInTheDocument()
    })

    it('shows budget percentage when budgetLimit is provided', () => {
      render(<CostTimelineChart data={mockData} budgetLimit={0.10} />)
      expect(screen.getByText('Budget Used')).toBeInTheDocument()
      // 0.06 / 0.10 = 60%
      expect(screen.getByText('60.0%')).toBeInTheDocument()
    })

    it('does not show budget percentage without budgetLimit', () => {
      render(<CostTimelineChart data={mockData} />)
      expect(screen.queryByText('Budget Used')).not.toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('renders empty message when no data', () => {
      render(<CostTimelineChart data={[]} />)
      expect(screen.getByText('No cost data available')).toBeInTheDocument()
    })

    it('renders custom empty message', () => {
      render(
        <CostTimelineChart data={[]} emptyMessage="No iterations recorded" />
      )
      expect(screen.getByText('No iterations recorded')).toBeInTheDocument()
    })

    it('does not render SVG when no data', () => {
      const { container } = render(<CostTimelineChart data={[]} />)
      const svg = container.querySelector('svg')
      expect(svg).not.toBeInTheDocument()
    })
  })

  describe('SVG rendering', () => {
    it('renders SVG element', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)
      const svg = container.querySelector('svg')
      expect(svg).toBeInTheDocument()
    })

    it('renders cumulative cost line path', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)
      const paths = container.querySelectorAll('path')
      // Should have at least 1 path for the cost line
      expect(paths.length).toBeGreaterThanOrEqual(1)
    })

    it('renders area fill when showFill is true', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} showFill={true} />
      )
      // Should have path with fill for area
      const paths = container.querySelectorAll('path')
      const hasFillPath = Array.from(paths).some(
        p => p.getAttribute('fill')?.includes('rgba')
      )
      expect(hasFillPath).toBe(true)
    })

    it('renders iteration cost bars when showIterationBars is true', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} showIterationBars={true} />
      )
      // Should have rect elements for bars
      const rects = container.querySelectorAll('rect')
      // At least 3 bar rects + 3 hitbox rects = 6+
      expect(rects.length).toBeGreaterThanOrEqual(6)
    })

    it('renders data point markers', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)
      const circles = container.querySelectorAll('circle')
      // Should have 3 circles for 3 data points
      expect(circles.length).toBe(3)
    })
  })

  describe('budget threshold lines', () => {
    it('renders budget limit line when budgetLimit is provided', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} budgetLimit={0.10} />
      )
      // Look for dashed lines (budget and warning thresholds)
      const dashedLines = container.querySelectorAll('line[stroke-dasharray]')
      expect(dashedLines.length).toBeGreaterThanOrEqual(1)
    })

    it('renders warning threshold line', () => {
      const { container } = render(
        <CostTimelineChart
          data={mockData}
          budgetLimit={0.10}
          warningThreshold={75}
        />
      )
      // Should have warning line
      const lines = container.querySelectorAll('line')
      expect(lines.length).toBeGreaterThanOrEqual(2) // grid lines + threshold lines
    })

    it('displays budget label', () => {
      render(<CostTimelineChart data={mockData} budgetLimit={0.10} />)
      expect(screen.getByText(/Budget: \$0\.10/)).toBeInTheDocument()
    })
  })

  describe('X-axis labels', () => {
    it('renders iteration labels for small datasets', () => {
      render(<CostTimelineChart data={mockData} />)
      // Should show iteration labels
      expect(screen.getByText('#1')).toBeInTheDocument()
      expect(screen.getByText('#2')).toBeInTheDocument()
      expect(screen.getByText('#3')).toBeInTheDocument()
    })

    it('renders custom labels when provided', () => {
      const dataWithLabels: CostDataPoint[] = mockData.map((d, i) => ({
        ...d,
        label: `Step ${i + 1}`,
      }))
      render(<CostTimelineChart data={dataWithLabels} />)
      expect(screen.getByText('Step 1')).toBeInTheDocument()
      expect(screen.getByText('Step 2')).toBeInTheDocument()
      expect(screen.getByText('Step 3')).toBeInTheDocument()
    })
  })

  describe('rolling window (maxPoints)', () => {
    it('limits displayed data to maxPoints', () => {
      const manyDataPoints: CostDataPoint[] = Array.from(
        { length: 50 },
        (_, i) => ({
          iteration: i + 1,
          timestamp: `2024-01-01T10:${i.toString().padStart(2, '0')}:00Z`,
          cumulativeCost: (i + 1) * 0.01,
          iterationCost: 0.01,
        })
      )

      render(<CostTimelineChart data={manyDataPoints} maxPoints={20} />)
      // Should show 20 as iteration count
      expect(screen.getByText('20')).toBeInTheDocument()
    })
  })

  describe('interaction', () => {
    it('shows tooltip on hover', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)

      // Find interactive rect elements (hitboxes)
      const hitboxRects = container.querySelectorAll('rect[fill="transparent"]')
      expect(hitboxRects.length).toBe(3)

      // Hover over first rect
      fireEvent.mouseEnter(hitboxRects[0])

      // Tooltip should appear
      const tooltip = container.querySelector('.cost-timeline-chart__tooltip')
      expect(tooltip).toBeInTheDocument()
    })

    it('tooltip shows iteration info', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)

      const hitboxRects = container.querySelectorAll('rect[fill="transparent"]')
      fireEvent.mouseEnter(hitboxRects[0])

      // Should show "Iteration 1" in tooltip
      expect(screen.getByText('Iteration 1')).toBeInTheDocument()
    })

    it('tooltip shows token info when available', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)

      const hitboxRects = container.querySelectorAll('rect[fill="transparent"]')
      fireEvent.mouseEnter(hitboxRects[0])

      // Should show token counts
      expect(screen.getByText(/1000 in \/ 500 out/)).toBeInTheDocument()
    })

    it('hides tooltip on mouse leave', () => {
      const { container } = render(<CostTimelineChart data={mockData} />)

      const hitboxRects = container.querySelectorAll('rect[fill="transparent"]')

      // Hover and then leave
      fireEvent.mouseEnter(hitboxRects[0])
      fireEvent.mouseLeave(hitboxRects[0])

      // Tooltip should be gone
      const tooltip = container.querySelector('.cost-timeline-chart__tooltip')
      expect(tooltip).not.toBeInTheDocument()
    })

    it('highlights iteration bar on hover', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} showIterationBars={true} />
      )

      const hitboxRects = container.querySelectorAll('rect[fill="transparent"]')
      fireEvent.mouseEnter(hitboxRects[0])

      // Bar should have increased opacity
      const bars = container.querySelectorAll('rect[opacity="0.8"]')
      expect(bars.length).toBe(1)
    })
  })

  describe('height prop', () => {
    it('respects custom height', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} height={200} />
      )
      const svg = container.querySelector('svg')
      expect(svg).toHaveAttribute('height', '200')
    })
  })

  describe('single data point', () => {
    it('renders single data point correctly', () => {
      const singlePoint: CostDataPoint[] = [
        {
          iteration: 1,
          timestamp: '2024-01-01T10:00:00Z',
          cumulativeCost: 0.05,
          iterationCost: 0.05,
          label: 'First',
        },
      ]

      render(<CostTimelineChart data={singlePoint} />)
      expect(screen.getByText('1')).toBeInTheDocument()
      expect(screen.getByText('First')).toBeInTheDocument()
      // Cost appears in both "Current Cost" and "Avg/Iteration" for single point
      const costElements = screen.getAllByText('$0.050')
      expect(costElements.length).toBeGreaterThan(0)
    })
  })

  describe('budget status colors', () => {
    it('shows green percentage when under 75%', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} budgetLimit={0.20} />
      )
      // 0.06 / 0.20 = 30% - should be green (#7ee787)
      const percentElement = container.querySelector('span[style*="color: rgb(126, 231, 135)"]')
      expect(percentElement).toBeInTheDocument()
    })

    it('shows orange percentage when between 75-100%', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} budgetLimit={0.07} />
      )
      // 0.06 / 0.07 = 85.7% - should be orange (#ffa657)
      const percentElement = container.querySelector('span[style*="color: rgb(255, 166, 87)"]')
      expect(percentElement).toBeInTheDocument()
    })

    it('shows red percentage when over budget', () => {
      const { container } = render(
        <CostTimelineChart data={mockData} budgetLimit={0.05} />
      )
      // 0.06 / 0.05 = 120% - should be red (#f85149)
      const percentElement = container.querySelector('span[style*="color: rgb(248, 81, 73)"]')
      expect(percentElement).toBeInTheDocument()
    })
  })
})

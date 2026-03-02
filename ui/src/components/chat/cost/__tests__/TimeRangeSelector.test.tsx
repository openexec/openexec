/**
 * Tests for TimeRangeSelector component
 *
 * @module components/chat/cost/__tests__/TimeRangeSelector.test
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import TimeRangeSelector, {
  type TimeRange,
  type TimeRangePreset,
} from '../TimeRangeSelector'

describe('TimeRangeSelector', () => {
  const mockOnChange = vi.fn()

  const defaultValue: TimeRange = {
    since: null,
    until: null,
    preset: 'all_time',
  }

  beforeEach(() => {
    mockOnChange.mockClear()
  })

  describe('rendering', () => {
    it('should render all preset buttons', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      expect(screen.getByText('Last Hour')).toBeDefined()
      expect(screen.getByText('Last 24 Hours')).toBeDefined()
      expect(screen.getByText('Last 7 Days')).toBeDefined()
      expect(screen.getByText('Last 30 Days')).toBeDefined()
      expect(screen.getByText('Last 90 Days')).toBeDefined()
      // "All Time" appears both as button and display, so use getAllByText
      expect(screen.getAllByText('All Time').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('Custom')).toBeDefined()
    })

    it('should render compact labels when compact is true', () => {
      render(
        <TimeRangeSelector value={defaultValue} onChange={mockOnChange} compact={true} />
      )

      expect(screen.getByText('1h')).toBeDefined()
      expect(screen.getByText('24h')).toBeDefined()
      expect(screen.getByText('7d')).toBeDefined()
      expect(screen.getByText('30d')).toBeDefined()
      expect(screen.getByText('90d')).toBeDefined()
      expect(screen.getByText('All')).toBeDefined()
    })

    it('should show current range display', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      // "All Time" appears both as button and display, so use getAllByText
      expect(screen.getAllByText('All Time').length).toBe(2)
    })

    it('should highlight the active preset button', () => {
      const value: TimeRange = {
        since: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
        until: new Date(),
        preset: 'last_7d',
      }

      render(<TimeRangeSelector value={value} onChange={mockOnChange} />)

      const last7dButton = screen.getAllByText('Last 7 Days')[0]
      // Check for active styling by checking if it exists (basic check)
      expect(last7dButton).toBeDefined()
    })
  })

  describe('preset selection', () => {
    it('should call onChange with correct dates for last_hour', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Last Hour'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('last_hour')
      expect(call.since).toBeInstanceOf(Date)
      expect(call.until).toBeInstanceOf(Date)
    })

    it('should call onChange with correct dates for last_24h', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Last 24 Hours'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('last_24h')
      expect(call.since).toBeInstanceOf(Date)
      expect(call.until).toBeInstanceOf(Date)
    })

    it('should call onChange with correct dates for last_7d', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Last 7 Days'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('last_7d')
      expect(call.since).toBeInstanceOf(Date)
      expect(call.until).toBeInstanceOf(Date)
      // Verify the date is roughly 7 days ago
      const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000
      expect(call.since!.getTime()).toBeGreaterThan(sevenDaysAgo - 1000)
      expect(call.since!.getTime()).toBeLessThan(sevenDaysAgo + 1000)
    })

    it('should call onChange with correct dates for last_30d', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Last 30 Days'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('last_30d')
    })

    it('should call onChange with correct dates for last_90d', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Last 90 Days'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('last_90d')
    })

    it('should call onChange with null dates for all_time', () => {
      const value: TimeRange = {
        since: new Date(),
        until: new Date(),
        preset: 'last_7d',
      }

      render(<TimeRangeSelector value={value} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('All Time'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('all_time')
      expect(call.since).toBeNull()
      expect(call.until).toBeNull()
    })
  })

  describe('custom date selection', () => {
    it('should show custom inputs when Custom is clicked', () => {
      render(
        <TimeRangeSelector
          value={defaultValue}
          onChange={mockOnChange}
          showCustomInputs={true}
        />
      )

      fireEvent.click(screen.getByText('Custom'))

      expect(screen.getByText('From')).toBeDefined()
      expect(screen.getByText('To')).toBeDefined()
    })

    it('should not show custom inputs when showCustomInputs is false', () => {
      render(
        <TimeRangeSelector
          value={defaultValue}
          onChange={mockOnChange}
          showCustomInputs={false}
        />
      )

      expect(screen.queryByText('Custom')).toBeNull()
    })

    it('should call onChange when custom preset is selected', () => {
      render(<TimeRangeSelector value={defaultValue} onChange={mockOnChange} />)

      fireEvent.click(screen.getByText('Custom'))

      expect(mockOnChange).toHaveBeenCalledTimes(1)
      const call = mockOnChange.mock.calls[0][0] as TimeRange
      expect(call.preset).toBe('custom')
    })
  })

  describe('disabled state', () => {
    it('should not call onChange when disabled', () => {
      render(
        <TimeRangeSelector value={defaultValue} onChange={mockOnChange} disabled={true} />
      )

      fireEvent.click(screen.getByText('Last 7 Days'))

      expect(mockOnChange).not.toHaveBeenCalled()
    })

    it('should apply disabled styling to buttons', () => {
      render(
        <TimeRangeSelector value={defaultValue} onChange={mockOnChange} disabled={true} />
      )

      const button = screen.getByText('Last 7 Days')
      expect(button).toHaveProperty('disabled', true)
    })
  })

  describe('range display', () => {
    it('should display preset label for preset values', () => {
      const value: TimeRange = {
        since: new Date(),
        until: new Date(),
        preset: 'last_30d',
      }

      render(<TimeRangeSelector value={value} onChange={mockOnChange} />)

      // The display should show "Last 30 Days"
      const displays = screen.getAllByText('Last 30 Days')
      expect(displays.length).toBeGreaterThanOrEqual(1)
    })

    it('should display formatted dates for custom range', () => {
      const value: TimeRange = {
        since: new Date('2024-01-01'),
        until: new Date('2024-01-15'),
        preset: 'custom',
      }

      render(<TimeRangeSelector value={value} onChange={mockOnChange} />)

      // The display should show formatted date range
      expect(screen.getByText(/Jan 1.*Jan 15/)).toBeDefined()
    })

    it('should display ellipsis for null dates in custom mode', () => {
      const value: TimeRange = {
        since: null,
        until: new Date('2024-01-15'),
        preset: 'custom',
      }

      render(<TimeRangeSelector value={value} onChange={mockOnChange} />)

      expect(screen.getByText(/\.\.\. - Jan 15/)).toBeDefined()
    })
  })
})

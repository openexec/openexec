/**
 * TimeRangeSelector Component
 *
 * Provides preset time range options and custom date range selection
 * for filtering usage data in charts and dashboards.
 *
 * @module components/chat/cost/TimeRangeSelector
 */

import React, { useState, useMemo, useCallback } from 'react'

/**
 * Preset time range options
 */
export type TimeRangePreset =
  | 'last_hour'
  | 'last_24h'
  | 'last_7d'
  | 'last_30d'
  | 'last_90d'
  | 'all_time'
  | 'custom'

/**
 * Time range value representing a date range
 */
export interface TimeRange {
  /** Start of the time range */
  since: Date | null
  /** End of the time range */
  until: Date | null
  /** Currently selected preset */
  preset: TimeRangePreset
}

/**
 * Props for TimeRangeSelector
 */
export interface TimeRangeSelectorProps {
  /** Current time range value */
  value: TimeRange
  /** Callback when time range changes */
  onChange: (range: TimeRange) => void
  /** Whether the selector is disabled */
  disabled?: boolean
  /** Show custom date inputs */
  showCustomInputs?: boolean
  /** Compact display mode */
  compact?: boolean
}

/**
 * Calculate date from preset
 */
function getDateFromPreset(preset: TimeRangePreset): { since: Date | null; until: Date | null } {
  const now = new Date()
  const until = now

  switch (preset) {
    case 'last_hour':
      return { since: new Date(now.getTime() - 60 * 60 * 1000), until }
    case 'last_24h':
      return { since: new Date(now.getTime() - 24 * 60 * 60 * 1000), until }
    case 'last_7d':
      return { since: new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000), until }
    case 'last_30d':
      return { since: new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000), until }
    case 'last_90d':
      return { since: new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000), until }
    case 'all_time':
      return { since: null, until: null }
    case 'custom':
      return { since: null, until: null }
    default:
      return { since: null, until: null }
  }
}

/**
 * Format date to local datetime-local input format
 */
function formatDateForInput(date: Date | null): string {
  if (!date) return ''
  const offset = date.getTimezoneOffset()
  const localDate = new Date(date.getTime() - offset * 60 * 1000)
  return localDate.toISOString().slice(0, 16)
}

/**
 * Preset configuration with labels
 */
const PRESETS: Array<{ value: TimeRangePreset; label: string; shortLabel: string }> = [
  { value: 'last_hour', label: 'Last Hour', shortLabel: '1h' },
  { value: 'last_24h', label: 'Last 24 Hours', shortLabel: '24h' },
  { value: 'last_7d', label: 'Last 7 Days', shortLabel: '7d' },
  { value: 'last_30d', label: 'Last 30 Days', shortLabel: '30d' },
  { value: 'last_90d', label: 'Last 90 Days', shortLabel: '90d' },
  { value: 'all_time', label: 'All Time', shortLabel: 'All' },
]

/**
 * TimeRangeSelector Component
 */
const TimeRangeSelector: React.FC<TimeRangeSelectorProps> = ({
  value,
  onChange,
  disabled = false,
  showCustomInputs = true,
  compact = false,
}) => {
  const [isCustomOpen, setIsCustomOpen] = useState(value.preset === 'custom')

  /**
   * Handle preset selection
   */
  const handlePresetChange = useCallback(
    (preset: TimeRangePreset) => {
      if (disabled) return

      if (preset === 'custom') {
        setIsCustomOpen(true)
        onChange({ ...value, preset: 'custom' })
      } else {
        setIsCustomOpen(false)
        const { since, until } = getDateFromPreset(preset)
        onChange({ since, until, preset })
      }
    },
    [disabled, onChange, value]
  )

  /**
   * Handle custom date changes
   */
  const handleCustomDateChange = useCallback(
    (field: 'since' | 'until', dateString: string) => {
      if (disabled) return

      const date = dateString ? new Date(dateString) : null
      onChange({
        ...value,
        [field]: date,
        preset: 'custom',
      })
    },
    [disabled, onChange, value]
  )

  /**
   * Format the current range for display
   */
  const rangeDisplay = useMemo(() => {
    if (value.preset !== 'custom') {
      const preset = PRESETS.find(p => p.value === value.preset)
      return preset?.label ?? 'All Time'
    }

    const formatDate = (d: Date | null): string => {
      if (!d) return '...'
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    }

    return `${formatDate(value.since)} - ${formatDate(value.until)}`
  }, [value])

  return (
    <div className="time-range-selector" style={styles.container}>
      {/* Preset buttons */}
      <div className="time-range-selector__presets" style={styles.presets}>
        {PRESETS.map(preset => (
          <button
            key={preset.value}
            onClick={() => handlePresetChange(preset.value)}
            disabled={disabled}
            style={{
              ...styles.presetButton,
              ...(value.preset === preset.value ? styles.presetButtonActive : {}),
              ...(disabled ? styles.presetButtonDisabled : {}),
            }}
            title={preset.label}
          >
            {compact ? preset.shortLabel : preset.label}
          </button>
        ))}
        {showCustomInputs && (
          <button
            onClick={() => handlePresetChange('custom')}
            disabled={disabled}
            style={{
              ...styles.presetButton,
              ...(value.preset === 'custom' ? styles.presetButtonActive : {}),
              ...(disabled ? styles.presetButtonDisabled : {}),
            }}
            title="Custom Range"
          >
            {compact ? '...' : 'Custom'}
          </button>
        )}
      </div>

      {/* Custom date inputs */}
      {showCustomInputs && isCustomOpen && (
        <div className="time-range-selector__custom" style={styles.customSection}>
          <div className="time-range-selector__input-group" style={styles.inputGroup}>
            <label style={styles.label}>From</label>
            <input
              type="datetime-local"
              value={formatDateForInput(value.since)}
              onChange={e => handleCustomDateChange('since', e.target.value)}
              disabled={disabled}
              style={styles.dateInput}
            />
          </div>
          <div className="time-range-selector__input-group" style={styles.inputGroup}>
            <label style={styles.label}>To</label>
            <input
              type="datetime-local"
              value={formatDateForInput(value.until)}
              onChange={e => handleCustomDateChange('until', e.target.value)}
              disabled={disabled}
              style={styles.dateInput}
            />
          </div>
        </div>
      )}

      {/* Current range display */}
      <div className="time-range-selector__display" style={styles.display}>
        <CalendarIcon />
        <span style={styles.displayText}>{rangeDisplay}</span>
      </div>
    </div>
  )
}

/**
 * Calendar icon component
 */
const CalendarIcon: React.FC = () => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
    <line x1="16" y1="2" x2="16" y2="6" />
    <line x1="8" y1="2" x2="8" y2="6" />
    <line x1="3" y1="10" x2="21" y2="10" />
  </svg>
)

/**
 * Styles
 */
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  presets: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '4px',
  },
  presetButton: {
    padding: '6px 10px',
    fontSize: '11px',
    fontWeight: 500,
    color: '#8b949e',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
    whiteSpace: 'nowrap',
  },
  presetButtonActive: {
    color: '#c9d1d9',
    backgroundColor: '#238636',
    borderColor: '#238636',
  },
  presetButtonDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
  customSection: {
    display: 'flex',
    gap: '12px',
    padding: '8px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
  },
  inputGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    flex: 1,
  },
  label: {
    fontSize: '10px',
    fontWeight: 500,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  dateInput: {
    padding: '6px 8px',
    fontSize: '12px',
    color: '#c9d1d9',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '4px',
    outline: 'none',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  display: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '11px',
    color: '#6e7681',
  },
  displayText: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
}

export default TimeRangeSelector

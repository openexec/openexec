/**
 * EventFilter Component
 *
 * Filter controls for the event list by type and kind.
 *
 * @module components/chat/events/EventFilter
 */

import React, { useCallback } from 'react'
import type { EventKind, EventFilters } from '../../../types/chat'

export interface EventFilterProps {
  /** Current filter settings */
  filters: EventFilters
  /** Callback when filters change */
  onChange: (filters: EventFilters) => void
}

const EVENT_KINDS: EventKind[] = [
  'lifecycle',
  'iteration',
  'llm',
  'tool',
  'context',
  'message',
  'gate',
  'signal',
  'cost',
  'session',
  'thrashing',
]

const KIND_LABELS: Record<EventKind, string> = {
  lifecycle: 'Lifecycle',
  iteration: 'Iteration',
  llm: 'LLM',
  tool: 'Tool',
  context: 'Context',
  message: 'Message',
  gate: 'Gate',
  signal: 'Signal',
  cost: 'Cost',
  session: 'Session',
  thrashing: 'Thrashing',
}

const KIND_COLORS: Record<EventKind, string> = {
  lifecycle: '#58a6ff',
  iteration: '#a371f7',
  llm: '#79c0ff',
  tool: '#ffa657',
  context: '#7ee787',
  message: '#c9d1d9',
  gate: '#ff7b72',
  signal: '#d2a8ff',
  cost: '#ffd33d',
  session: '#a5d6ff',
  thrashing: '#f85149',
}

const EventFilter: React.FC<EventFilterProps> = ({ filters, onChange }) => {
  // Toggle a kind filter
  const toggleKind = useCallback(
    (kind: EventKind) => {
      const currentKinds = filters.kinds || []
      const newKinds = currentKinds.includes(kind)
        ? currentKinds.filter((k) => k !== kind)
        : [...currentKinds, kind]

      onChange({
        ...filters,
        kinds: newKinds.length > 0 ? newKinds : undefined,
      })
    },
    [filters, onChange]
  )

  // Toggle include errors
  const toggleErrors = useCallback(() => {
    onChange({
      ...filters,
      includeErrors: !filters.includeErrors,
    })
  }, [filters, onChange])

  // Clear all filters
  const clearFilters = useCallback(() => {
    onChange({})
  }, [onChange])

  // Check if any filters are active
  const hasActiveFilters =
    (filters.kinds && filters.kinds.length > 0) ||
    filters.includeErrors === false

  // Check if a kind is selected (when no filters, all are shown)
  const isKindSelected = (kind: EventKind): boolean => {
    if (!filters.kinds || filters.kinds.length === 0) return true
    return filters.kinds.includes(kind)
  }

  return (
    <div className="event-filter" style={styles.container}>
      {/* Filter header */}
      <div className="event-filter__header" style={styles.header}>
        <span style={styles.title}>Filter events</span>
        {hasActiveFilters && (
          <button
            className="event-filter__clear"
            style={styles.clearBtn}
            onClick={clearFilters}
            aria-label="Clear all filters"
          >
            Clear
          </button>
        )}
      </div>

      {/* Kind filters */}
      <div className="event-filter__kinds" style={styles.kinds}>
        {EVENT_KINDS.map((kind) => (
          <button
            key={kind}
            className="event-filter__kind-btn"
            style={{
              ...styles.kindBtn,
              backgroundColor: isKindSelected(kind) ? KIND_COLORS[kind] : 'transparent',
              color: isKindSelected(kind) ? '#0d1117' : KIND_COLORS[kind],
              borderColor: KIND_COLORS[kind],
              opacity: isKindSelected(kind) ? 1 : 0.6,
            }}
            onClick={() => toggleKind(kind)}
            aria-pressed={isKindSelected(kind)}
            title={`${isKindSelected(kind) ? 'Hide' : 'Show'} ${KIND_LABELS[kind]} events`}
          >
            {KIND_LABELS[kind]}
          </button>
        ))}
      </div>

      {/* Error filter */}
      <div className="event-filter__options" style={styles.options}>
        <label className="event-filter__checkbox" style={styles.checkbox}>
          <input
            type="checkbox"
            checked={filters.includeErrors !== false}
            onChange={toggleErrors}
            style={styles.input}
          />
          <span style={styles.checkboxLabel}>Show errors</span>
        </label>
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    padding: '12px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: '10px',
  },
  title: {
    fontSize: '11px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  clearBtn: {
    backgroundColor: 'transparent',
    border: 'none',
    color: '#58a6ff',
    fontSize: '11px',
    cursor: 'pointer',
    padding: '2px 6px',
  },
  kinds: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '4px',
    marginBottom: '10px',
  },
  kindBtn: {
    padding: '2px 8px',
    fontSize: '10px',
    fontWeight: 500,
    border: '1px solid',
    borderRadius: '10px',
    cursor: 'pointer',
    transition: 'all 0.15s ease',
  },
  options: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  checkbox: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    cursor: 'pointer',
  },
  input: {
    width: '14px',
    height: '14px',
    accentColor: '#58a6ff',
    cursor: 'pointer',
  },
  checkboxLabel: {
    fontSize: '11px',
    color: '#8b949e',
  },
}

export default EventFilter

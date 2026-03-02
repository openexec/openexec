/**
 * EventPanel Component
 *
 * Collapsible panel showing loop events with filtering.
 * Provides real-time event streaming and filtering capabilities.
 *
 * @module components/chat/events/EventPanel
 */

import React, { useState, useCallback, useMemo } from 'react'
import type { LoopEvent, EventFilters } from '../../../types/chat'
import EventFilter from './EventFilter'
import EventList from './EventList'

export interface EventPanelProps {
  /** List of events to display */
  events: LoopEvent[]
  /** Initial filter settings */
  defaultFilters?: EventFilters
  /** Whether the agent loop is currently running */
  isLive?: boolean
  /** Callback when filters change */
  onFilterChange?: (filters: EventFilters) => void
  /** Callback to clear events */
  onClear?: () => void
}

const EventPanel: React.FC<EventPanelProps> = ({
  events,
  defaultFilters = {},
  isLive = false,
  onFilterChange,
  onClear,
}) => {
  const [filters, setFilters] = useState<EventFilters>(defaultFilters)
  const [showFilters, setShowFilters] = useState(false)

  // Handle filter changes
  const handleFilterChange = useCallback(
    (newFilters: EventFilters) => {
      setFilters(newFilters)
      onFilterChange?.(newFilters)
    },
    [onFilterChange]
  )

  // Toggle filter panel visibility
  const toggleFilters = useCallback(() => {
    setShowFilters((prev) => !prev)
  }, [])

  // Filter events based on current filters
  const filteredEvents = useMemo(() => {
    return events.filter((event) => {
      // Filter by kind
      if (filters.kinds && filters.kinds.length > 0) {
        if (!filters.kinds.includes(event.kind)) {
          return false
        }
      }

      // Filter by type
      if (filters.types && filters.types.length > 0) {
        if (!filters.types.includes(event.type)) {
          return false
        }
      }

      // Filter errors
      if (filters.includeErrors === false && event.error) {
        return false
      }

      // Filter by time range
      if (filters.since) {
        const eventTime = new Date(event.timestamp).getTime()
        const sinceTime = new Date(filters.since).getTime()
        if (eventTime < sinceTime) {
          return false
        }
      }

      if (filters.until) {
        const eventTime = new Date(event.timestamp).getTime()
        const untilTime = new Date(filters.until).getTime()
        if (eventTime > untilTime) {
          return false
        }
      }

      return true
    })
  }, [events, filters])

  // Count of filtered vs total events
  const filterSummary =
    filteredEvents.length !== events.length
      ? `${filteredEvents.length} / ${events.length}`
      : `${events.length}`

  return (
    <div className="event-panel" style={styles.container}>
      {/* Panel header */}
      <div className="event-panel__header" style={styles.header}>
        <div className="event-panel__title-row" style={styles.titleRow}>
          <h3 style={styles.title}>Events</h3>
          <span style={styles.count}>{filterSummary}</span>
        </div>

        <div className="event-panel__actions" style={styles.actions}>
          {/* Filter toggle button */}
          <button
            className="event-panel__filter-btn"
            style={{
              ...styles.actionBtn,
              backgroundColor: showFilters ? '#21262d' : 'transparent',
            }}
            onClick={toggleFilters}
            aria-label={showFilters ? 'Hide filters' : 'Show filters'}
            aria-pressed={showFilters}
            title="Toggle filters"
          >
            <FilterIcon />
          </button>

          {/* Clear button */}
          {onClear && events.length > 0 && (
            <button
              className="event-panel__clear-btn"
              style={styles.actionBtn}
              onClick={onClear}
              aria-label="Clear events"
              title="Clear all events"
            >
              <ClearIcon />
            </button>
          )}
        </div>
      </div>

      {/* Filter panel (collapsible) */}
      {showFilters && (
        <EventFilter filters={filters} onChange={handleFilterChange} />
      )}

      {/* Event list */}
      <EventList
        events={filteredEvents}
        autoScroll={true}
        isLive={isLive}
      />
    </div>
  )
}

// Icon components
const FilterIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
  </svg>
)

const ClearIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="3 6 5 6 21 6" />
    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    backgroundColor: '#161b22',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 12px 10px 12px',
    borderBottom: '1px solid #30363d',
    flexShrink: 0,
  },
  titleRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  title: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  count: {
    fontSize: '11px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    padding: '1px 6px',
    borderRadius: '10px',
  },
  actions: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  actionBtn: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '28px',
    height: '28px',
    backgroundColor: 'transparent',
    border: 'none',
    borderRadius: '4px',
    color: '#8b949e',
    cursor: 'pointer',
  },
}

export default EventPanel

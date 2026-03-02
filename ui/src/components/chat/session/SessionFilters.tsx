/**
 * SessionFilters Component
 *
 * Filter controls for the session list.
 * Includes search input and status/sort dropdowns.
 *
 * @module components/chat/session/SessionFilters
 */

import React, { useCallback, useState } from 'react'
import type { SessionFilters as SessionFiltersType, SessionStatus } from '../../../types/chat'

export interface SessionFiltersProps {
  /** Current filter values */
  filters: SessionFiltersType
  /** Callback when filters change */
  onChange?: (filters: SessionFiltersType) => void
}

const SessionFilters: React.FC<SessionFiltersProps> = ({
  filters,
  onChange,
}) => {
  const [showAdvanced, setShowAdvanced] = useState(false)

  // Handle search input change with debounce
  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    onChange?.({
      ...filters,
      search: e.target.value || undefined,
    })
  }, [filters, onChange])

  // Handle status filter change
  const handleStatusChange = useCallback((e: React.ChangeEvent<HTMLSelectElement>) => {
    const value = e.target.value as SessionStatus | ''
    onChange?.({
      ...filters,
      status: value || undefined,
    })
  }, [filters, onChange])

  // Handle sort change
  const handleSortChange = useCallback((e: React.ChangeEvent<HTMLSelectElement>) => {
    const [sortBy, sortOrder] = e.target.value.split(':') as [SessionFiltersType['sortBy'], SessionFiltersType['sortOrder']]
    onChange?.({
      ...filters,
      sortBy: sortBy || undefined,
      sortOrder: sortOrder || undefined,
    })
  }, [filters, onChange])

  // Clear all filters
  const handleClear = useCallback(() => {
    onChange?.({})
  }, [onChange])

  // Check if any filters are active
  const hasActiveFilters = filters.search || filters.status || filters.sortBy

  // Get current sort value for select
  const getSortValue = (): string => {
    if (!filters.sortBy) return 'updated_at:desc'
    return `${filters.sortBy}:${filters.sortOrder || 'desc'}`
  }

  return (
    <div className="session-filters" style={styles.container}>
      {/* Search input */}
      <div className="session-filters__search" style={styles.searchContainer}>
        <SearchIcon />
        <input
          type="text"
          placeholder="Search sessions..."
          value={filters.search || ''}
          onChange={handleSearchChange}
          style={styles.searchInput}
        />
        {filters.search && (
          <button
            onClick={() => onChange?.({ ...filters, search: undefined })}
            style={styles.clearButton}
            title="Clear search"
          >
            <CloseIcon />
          </button>
        )}
      </div>

      {/* Toggle advanced filters */}
      <button
        className="session-filters__toggle"
        style={styles.toggleButton}
        onClick={() => setShowAdvanced(!showAdvanced)}
        aria-expanded={showAdvanced}
      >
        <FilterIcon />
        {hasActiveFilters && <span style={styles.filterBadge} />}
      </button>

      {/* Advanced filters dropdown */}
      {showAdvanced && (
        <div className="session-filters__advanced" style={styles.advanced}>
          {/* Status filter */}
          <div style={styles.filterGroup}>
            <label style={styles.label}>Status</label>
            <select
              value={filters.status || ''}
              onChange={handleStatusChange}
              style={styles.select}
            >
              <option value="">All</option>
              <option value="active">Active</option>
              <option value="paused">Paused</option>
              <option value="archived">Archived</option>
            </select>
          </div>

          {/* Sort options */}
          <div style={styles.filterGroup}>
            <label style={styles.label}>Sort by</label>
            <select
              value={getSortValue()}
              onChange={handleSortChange}
              style={styles.select}
            >
              <option value="updated_at:desc">Recently Updated</option>
              <option value="updated_at:asc">Oldest Updated</option>
              <option value="created_at:desc">Recently Created</option>
              <option value="created_at:asc">Oldest Created</option>
              <option value="title:asc">Title A-Z</option>
              <option value="title:desc">Title Z-A</option>
            </select>
          </div>

          {/* Clear filters */}
          {hasActiveFilters && (
            <button
              onClick={handleClear}
              style={styles.clearAllButton}
            >
              Clear all filters
            </button>
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const SearchIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#8b949e" strokeWidth="2">
    <circle cx="11" cy="11" r="8" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
  </svg>
)

const FilterIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  searchContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '6px 10px',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
  },
  searchInput: {
    flex: 1,
    border: 'none',
    background: 'none',
    outline: 'none',
    color: '#c9d1d9',
    fontSize: '13px',
    padding: 0,
  },
  clearButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    color: '#8b949e',
    padding: '2px',
    borderRadius: '4px',
  },
  toggleButton: {
    position: 'absolute',
    right: '12px',
    top: '12px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '28px',
    height: '28px',
    background: 'none',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
    color: '#8b949e',
  },
  filterBadge: {
    position: 'absolute',
    top: '-2px',
    right: '-2px',
    width: '8px',
    height: '8px',
    backgroundColor: '#58a6ff',
    borderRadius: '50%',
  },
  advanced: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    padding: '12px',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
  },
  filterGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  label: {
    fontSize: '11px',
    fontWeight: 500,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  select: {
    padding: '6px 8px',
    fontSize: '13px',
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
    outline: 'none',
  },
  clearAllButton: {
    padding: '6px 12px',
    fontSize: '12px',
    color: '#58a6ff',
    backgroundColor: 'transparent',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
    marginTop: '4px',
  },
}

export default SessionFilters

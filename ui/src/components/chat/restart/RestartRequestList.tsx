/**
 * RestartRequestList Component
 * List view for multiple restart requests with filtering.
 * @module components/chat/restart/RestartRequestList
 */
import React, { useState, useMemo } from 'react'
import type { RestartRequest, RestartStatus } from '../../../types/restart'
import RestartRequestCard from './RestartRequestCard'

export interface RestartRequestListProps {
  /** List of restart requests */
  requests: RestartRequest[]
  /** Callback when a request is clicked */
  onRequestClick?: (request: RestartRequest) => void
  /** Callback when quick approve is clicked */
  onQuickApprove?: (request: RestartRequest) => void
  /** Callback when quick reject is clicked */
  onQuickReject?: (request: RestartRequest) => void
  /** Whether to show filters */
  showFilters?: boolean
  /** Title for the list */
  title?: string
  /** Empty state message */
  emptyMessage?: string
}

const RestartRequestList: React.FC<RestartRequestListProps> = ({
  requests,
  onRequestClick,
  onQuickApprove,
  onQuickReject,
  showFilters = true,
  title = 'Restart Requests',
  emptyMessage = 'No restart requests',
}) => {
  const [statusFilter, setStatusFilter] = useState<RestartStatus | 'all'>('all')

  // Filter requests
  const filteredRequests = useMemo(() => {
    if (statusFilter === 'all') return requests
    return requests.filter((r) => r.status === statusFilter)
  }, [requests, statusFilter])

  // Count by status for filter badges
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = { all: requests.length }
    requests.forEach((r) => {
      counts[r.status] = (counts[r.status] || 0) + 1
    })
    return counts
  }, [requests])

  // Status filter options
  const statusOptions: { value: RestartStatus | 'all'; label: string; color?: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'pending', label: 'Pending', color: '#f0883e' },
    { value: 'approved', label: 'Approved', color: '#238636' },
    { value: 'in_progress', label: 'In Progress', color: '#58a6ff' },
    { value: 'complete', label: 'Complete', color: '#238636' },
    { value: 'failed', label: 'Failed', color: '#da3633' },
    { value: 'rejected', label: 'Rejected', color: '#da3633' },
    { value: 'cancelled', label: 'Cancelled', color: '#8b949e' },
  ]

  return (
    <div className="restart-request-list" style={styles.container}>
      {/* Header */}
      <div className="restart-request-list__header" style={styles.header}>
        <h3 className="restart-request-list__title" style={styles.title}>
          <RestartIcon />
          {title}
          <span style={styles.count}>({filteredRequests.length})</span>
        </h3>
      </div>

      {/* Filters */}
      {showFilters && (
        <div className="restart-request-list__filters" style={styles.filters}>
          {statusOptions.map((option) => {
            const count = statusCounts[option.value] || 0
            if (option.value !== 'all' && count === 0) return null

            return (
              <button
                key={option.value}
                className={`restart-request-list__filter ${statusFilter === option.value ? 'restart-request-list__filter--active' : ''}`}
                style={{
                  ...styles.filterButton,
                  ...(statusFilter === option.value ? styles.filterButtonActive : {}),
                  ...(option.color && statusFilter === option.value
                    ? { borderColor: option.color, color: option.color }
                    : {}),
                }}
                onClick={() => setStatusFilter(option.value)}
              >
                {option.label}
                {count > 0 && (
                  <span style={styles.filterCount}>{count}</span>
                )}
              </button>
            )
          })}
        </div>
      )}

      {/* List */}
      <div className="restart-request-list__content" style={styles.content}>
        {filteredRequests.length === 0 ? (
          <div className="restart-request-list__empty" style={styles.empty}>
            <EmptyIcon />
            <span>{emptyMessage}</span>
          </div>
        ) : (
          <div className="restart-request-list__items" style={styles.items}>
            {filteredRequests.map((request) => (
              <RestartRequestCard
                key={request.id}
                request={request}
                onClick={onRequestClick ? () => onRequestClick(request) : undefined}
                onQuickApprove={onQuickApprove ? () => onQuickApprove(request) : undefined}
                onQuickReject={onQuickReject ? () => onQuickReject(request) : undefined}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// Icon components
const RestartIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6" />
    <path d="M1 20v-6h6" />
    <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
  </svg>
)

const EmptyIcon: React.FC = () => (
  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
    <circle cx="12" cy="12" r="10" />
    <path d="M16 16s-1.5-2-4-2-4 2-4 2" />
    <line x1="9" y1="9" x2="9.01" y2="9" />
    <line x1="15" y1="9" x2="15.01" y2="9" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
  },
  header: {
    padding: '16px',
    borderBottom: '1px solid #21262d',
  },
  title: {
    margin: 0,
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  count: {
    fontSize: '12px',
    fontWeight: 400,
    color: '#8b949e',
  },
  filters: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '8px',
    padding: '12px 16px',
    borderBottom: '1px solid #21262d',
    backgroundColor: '#0d1117',
  },
  filterButton: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '4px 10px',
    fontSize: '12px',
    color: '#8b949e',
    backgroundColor: 'transparent',
    border: '1px solid #30363d',
    borderRadius: '20px',
    cursor: 'pointer',
    transition: 'all 0.2s',
  },
  filterButtonActive: {
    backgroundColor: '#21262d',
    color: '#c9d1d9',
    borderColor: '#c9d1d9',
  },
  filterCount: {
    fontSize: '10px',
    fontWeight: 500,
    padding: '0 4px',
    backgroundColor: 'rgba(255, 255, 255, 0.1)',
    borderRadius: '8px',
  },
  content: {
    flex: 1,
    overflow: 'auto',
  },
  items: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '16px',
  },
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '12px',
    padding: '48px 16px',
    color: '#484f58',
    textAlign: 'center',
    fontSize: '14px',
  },
}

export default RestartRequestList

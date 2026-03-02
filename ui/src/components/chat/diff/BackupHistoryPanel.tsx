/**
 * BackupHistoryPanel Component
 * Panel component for displaying a list of backups with filtering and rollback actions.
 * @module components/chat/diff/BackupHistoryPanel
 */
import React, { useState, useMemo, useCallback } from 'react'
import type { BackupMetadata, BackupStats } from '../../../types/backup'
import { formatFileSize, formatTimeAgo } from '../../../types/backup'
import BackupCard from './BackupCard'

export interface BackupHistoryPanelProps {
  /** List of backups to display */
  backups: BackupMetadata[]
  /** Overall backup statistics */
  stats?: BackupStats
  /** Callback when a backup is selected for restore */
  onRestore?: (backup: BackupMetadata) => void
  /** Callback when a backup is deleted */
  onDelete?: (backup: BackupMetadata) => void
  /** Callback when a backup is selected for viewing details */
  onSelect?: (backup: BackupMetadata) => void
  /** Currently selected backup ID */
  selectedBackupId?: string
  /** Whether actions are disabled */
  disabled?: boolean
  /** Whether to show in compact mode */
  compact?: boolean
  /** Maximum height of the panel (with scroll) */
  maxHeight?: string | number
  /** Title for the panel */
  title?: string
  /** Whether to show the search/filter bar */
  showFilter?: boolean
  /** Whether to show statistics */
  showStats?: boolean
  /** Loading state */
  isLoading?: boolean
  /** Error message */
  error?: string
}

type SortOption = 'newest' | 'oldest' | 'largest' | 'smallest' | 'name'
type FilterOption = 'all' | 'restored' | 'not_restored'

const BackupHistoryPanel: React.FC<BackupHistoryPanelProps> = ({
  backups,
  stats,
  onRestore,
  onDelete,
  onSelect,
  selectedBackupId,
  disabled = false,
  compact = false,
  maxHeight = '400px',
  title = 'Backup History',
  showFilter = true,
  showStats = true,
  isLoading = false,
  error,
}) => {
  const [searchQuery, setSearchQuery] = useState('')
  const [sortBy, setSortBy] = useState<SortOption>('newest')
  const [filterBy, setFilterBy] = useState<FilterOption>('all')

  // Filter and sort backups
  const filteredBackups = useMemo(() => {
    let result = [...backups]

    // Apply search filter
    if (searchQuery) {
      const query = searchQuery.toLowerCase()
      result = result.filter(
        (backup) =>
          backup.originalPath.toLowerCase().includes(query) ||
          backup.id.toLowerCase().includes(query) ||
          (backup.sessionId && backup.sessionId.toLowerCase().includes(query))
      )
    }

    // Apply restored filter
    if (filterBy === 'restored') {
      result = result.filter((backup) => backup.restored)
    } else if (filterBy === 'not_restored') {
      result = result.filter((backup) => !backup.restored)
    }

    // Apply sorting
    result.sort((a, b) => {
      switch (sortBy) {
        case 'newest':
          return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
        case 'oldest':
          return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
        case 'largest':
          return b.size - a.size
        case 'smallest':
          return a.size - b.size
        case 'name':
          return a.originalPath.localeCompare(b.originalPath)
        default:
          return 0
      }
    })

    return result
  }, [backups, searchQuery, sortBy, filterBy])

  // Determine which backup is the latest for each file
  const latestBackupIds = useMemo(() => {
    const latestByFile: Record<string, string> = {}
    const sortedByTime = [...backups].sort(
      (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    )
    for (const backup of sortedByTime) {
      if (!latestByFile[backup.originalPath]) {
        latestByFile[backup.originalPath] = backup.id
      }
    }
    return new Set(Object.values(latestByFile))
  }, [backups])

  // Handle search input
  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(e.target.value)
  }, [])

  // Clear search
  const handleClearSearch = useCallback(() => {
    setSearchQuery('')
  }, [])

  return (
    <div className="backup-history-panel" style={styles.container}>
      {/* Header */}
      <div className="backup-history-panel__header" style={styles.header}>
        <div style={styles.headerLeft}>
          <HistoryIcon />
          <h3 style={styles.title}>{title}</h3>
          <span style={styles.count}>
            {filteredBackups.length} backup{filteredBackups.length !== 1 ? 's' : ''}
          </span>
        </div>
      </div>

      {/* Stats bar */}
      {showStats && stats && (
        <div className="backup-history-panel__stats" style={styles.statsBar}>
          <div style={styles.statItem}>
            <span style={styles.statLabel}>Total</span>
            <span style={styles.statValue}>{stats.totalBackups}</span>
          </div>
          <div style={styles.statItem}>
            <span style={styles.statLabel}>Files</span>
            <span style={styles.statValue}>{stats.totalFiles}</span>
          </div>
          <div style={styles.statItem}>
            <span style={styles.statLabel}>Size</span>
            <span style={styles.statValue}>{formatFileSize(stats.totalSizeBytes)}</span>
          </div>
          {stats.oldestBackup && (
            <div style={styles.statItem}>
              <span style={styles.statLabel}>Oldest</span>
              <span style={styles.statValue}>{formatTimeAgo(stats.oldestBackup)}</span>
            </div>
          )}
        </div>
      )}

      {/* Filter bar */}
      {showFilter && (
        <div className="backup-history-panel__filter" style={styles.filterBar}>
          {/* Search input */}
          <div style={styles.searchContainer}>
            <SearchIcon />
            <input
              type="text"
              placeholder="Search backups..."
              value={searchQuery}
              onChange={handleSearchChange}
              style={styles.searchInput}
              disabled={disabled}
            />
            {searchQuery && (
              <button
                onClick={handleClearSearch}
                style={styles.clearButton}
                aria-label="Clear search"
              >
                <ClearIcon />
              </button>
            )}
          </div>

          {/* Sort select */}
          <div style={styles.selectContainer}>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as SortOption)}
              style={styles.select}
              disabled={disabled}
            >
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="largest">Largest first</option>
              <option value="smallest">Smallest first</option>
              <option value="name">By name</option>
            </select>
          </div>

          {/* Filter select */}
          <div style={styles.selectContainer}>
            <select
              value={filterBy}
              onChange={(e) => setFilterBy(e.target.value as FilterOption)}
              style={styles.select}
              disabled={disabled}
            >
              <option value="all">All backups</option>
              <option value="not_restored">Not restored</option>
              <option value="restored">Restored</option>
            </select>
          </div>
        </div>
      )}

      {/* Error display */}
      {error && (
        <div className="backup-history-panel__error" style={styles.errorContainer}>
          <ErrorIcon />
          <span>{error}</span>
        </div>
      )}

      {/* Loading state */}
      {isLoading && (
        <div className="backup-history-panel__loading" style={styles.loadingContainer}>
          <LoadingSpinner />
          <span>Loading backups...</span>
        </div>
      )}

      {/* Backup list */}
      {!isLoading && (
        <div
          className="backup-history-panel__list"
          style={{
            ...styles.list,
            maxHeight: maxHeight,
          }}
        >
          {filteredBackups.length === 0 ? (
            <div className="backup-history-panel__empty" style={styles.emptyState}>
              <EmptyIcon />
              <p style={styles.emptyTitle}>
                {searchQuery ? 'No backups match your search' : 'No backups available'}
              </p>
              <p style={styles.emptyDescription}>
                {searchQuery
                  ? 'Try adjusting your search or filters'
                  : 'Backups are created automatically when files are modified'}
              </p>
            </div>
          ) : (
            <div style={styles.listContent}>
              {filteredBackups.map((backup) => (
                <BackupCard
                  key={backup.id}
                  backup={backup}
                  isLatest={latestBackupIds.has(backup.id)}
                  onClick={onSelect ? () => onSelect(backup) : undefined}
                  onRestore={onRestore ? () => onRestore(backup) : undefined}
                  onDelete={onDelete ? () => onDelete(backup) : undefined}
                  compact={compact}
                  disabled={disabled}
                  selected={selectedBackupId === backup.id}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const HistoryIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const SearchIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="11" cy="11" r="8" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
  </svg>
)

const ClearIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="15" y1="9" x2="9" y2="15" />
    <line x1="9" y1="9" x2="15" y2="15" />
  </svg>
)

const LoadingSpinner: React.FC = () => (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

const EmptyIcon: React.FC = () => (
  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
    <path d="M3 3v5h5" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#0d1117',
    borderRadius: '8px',
    border: '1px solid #30363d',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#161b22',
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    color: '#f0883e',
  },
  title: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  count: {
    fontSize: '12px',
    color: '#8b949e',
    padding: '2px 8px',
    backgroundColor: '#21262d',
    borderRadius: '10px',
  },
  statsBar: {
    display: 'flex',
    gap: '16px',
    padding: '10px 16px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
  },
  statItem: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  statLabel: {
    fontSize: '10px',
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  statValue: {
    fontSize: '13px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  filterBar: {
    display: 'flex',
    gap: '10px',
    padding: '10px 16px',
    backgroundColor: '#0d1117',
    borderBottom: '1px solid #21262d',
  },
  searchContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flex: 1,
    padding: '6px 10px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '6px',
    color: '#8b949e',
  },
  searchInput: {
    flex: 1,
    backgroundColor: 'transparent',
    border: 'none',
    color: '#c9d1d9',
    fontSize: '13px',
    outline: 'none',
  },
  clearButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '20px',
    height: '20px',
    padding: 0,
    backgroundColor: 'transparent',
    border: 'none',
    color: '#8b949e',
    cursor: 'pointer',
    borderRadius: '4px',
  },
  selectContainer: {
    display: 'flex',
    alignItems: 'center',
  },
  select: {
    padding: '6px 10px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '6px',
    color: '#c9d1d9',
    fontSize: '12px',
    cursor: 'pointer',
    outline: 'none',
  },
  errorContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '12px 16px',
    backgroundColor: 'rgba(248, 81, 73, 0.1)',
    borderBottom: '1px solid rgba(248, 81, 73, 0.4)',
    color: '#f85149',
    fontSize: '13px',
  },
  loadingContainer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '10px',
    padding: '32px 16px',
    color: '#8b949e',
    fontSize: '13px',
  },
  list: {
    flex: 1,
    overflowY: 'auto',
    overflowX: 'hidden',
  },
  listContent: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '12px',
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '48px 24px',
    color: '#8b949e',
    textAlign: 'center',
  },
  emptyTitle: {
    margin: '16px 0 8px',
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  emptyDescription: {
    margin: 0,
    fontSize: '13px',
    color: '#8b949e',
  },
}

export default BackupHistoryPanel

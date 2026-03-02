/**
 * DiffStats Component
 * Displays statistics about a unified diff patch.
 * @module components/chat/diff/DiffStats
 */
import React from 'react'
import type { PatchStats } from '../../../types/diff'

export interface DiffStatsProps {
  /** Patch statistics */
  stats: PatchStats
  /** Compact mode for inline display */
  compact?: boolean
}

const DiffStats: React.FC<DiffStatsProps> = ({ stats, compact = false }) => {
  if (compact) {
    return (
      <div className="diff-stats diff-stats--compact" style={styles.compactContainer}>
        <span style={styles.filesCount}>
          {stats.filesChanged} {stats.filesChanged === 1 ? 'file' : 'files'}
        </span>
        <span style={styles.additions}>+{stats.additions}</span>
        <span style={styles.deletions}>-{stats.deletions}</span>
      </div>
    )
  }

  return (
    <div className="diff-stats" style={styles.container}>
      <div style={styles.statItem}>
        <FilesIcon />
        <span style={styles.statLabel}>Files changed:</span>
        <span style={styles.statValue}>{stats.filesChanged}</span>
      </div>
      <div style={styles.statItem}>
        <AddIcon />
        <span style={styles.statLabel}>Additions:</span>
        <span style={{ ...styles.statValue, color: '#3fb950' }}>+{stats.additions}</span>
      </div>
      <div style={styles.statItem}>
        <RemoveIcon />
        <span style={styles.statLabel}>Deletions:</span>
        <span style={{ ...styles.statValue, color: '#f85149' }}>-{stats.deletions}</span>
      </div>
      {stats.hunks > 0 && (
        <div style={styles.statItem}>
          <HunkIcon />
          <span style={styles.statLabel}>Hunks:</span>
          <span style={styles.statValue}>{stats.hunks}</span>
        </div>
      )}

      {/* Visual bar representation */}
      {(stats.additions > 0 || stats.deletions > 0) && (
        <div style={styles.barContainer}>
          <DiffBar additions={stats.additions} deletions={stats.deletions} />
        </div>
      )}
    </div>
  )
}

// Diff bar visualization component
interface DiffBarProps {
  additions: number
  deletions: number
}

const DiffBar: React.FC<DiffBarProps> = ({ additions, deletions }) => {
  const total = additions + deletions
  if (total === 0) return null

  const addWidth = (additions / total) * 100
  const delWidth = (deletions / total) * 100

  return (
    <div style={styles.bar}>
      {additions > 0 && (
        <div
          style={{
            ...styles.barSegment,
            width: `${addWidth}%`,
            backgroundColor: '#238636',
          }}
          title={`${additions} additions`}
        />
      )}
      {deletions > 0 && (
        <div
          style={{
            ...styles.barSegment,
            width: `${delWidth}%`,
            backgroundColor: '#da3633',
          }}
          title={`${deletions} deletions`}
        />
      )}
    </div>
  )
}

// Icon components
const FilesIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
  </svg>
)

const AddIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#3fb950" strokeWidth="2">
    <line x1="12" y1="5" x2="12" y2="19" />
    <line x1="5" y1="12" x2="19" y2="12" />
  </svg>
)

const RemoveIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f85149" strokeWidth="2">
    <line x1="5" y1="12" x2="19" y2="12" />
  </svg>
)

const HunkIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="3" y="3" width="18" height="18" rx="2" />
    <line x1="3" y1="9" x2="21" y2="9" />
    <line x1="3" y1="15" x2="21" y2="15" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '12px',
    backgroundColor: '#161b22',
    borderRadius: '6px',
    border: '1px solid #30363d',
  },
  compactContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '12px',
  },
  filesCount: {
    color: '#8b949e',
  },
  additions: {
    color: '#3fb950',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  deletions: {
    color: '#f85149',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  statItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#8b949e',
  },
  statLabel: {
    fontSize: '12px',
  },
  statValue: {
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  barContainer: {
    marginTop: '4px',
  },
  bar: {
    display: 'flex',
    height: '8px',
    borderRadius: '4px',
    overflow: 'hidden',
    backgroundColor: '#30363d',
  },
  barSegment: {
    height: '100%',
  },
}

export default DiffStats

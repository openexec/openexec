/**
 * DiffFileCard Component
 * Displays a single file's diff with collapsible hunks.
 * @module components/chat/diff/DiffFileCard
 */
import React, { useState, useMemo } from 'react'
import type { PatchFile, LineSelectInfo } from '../../../types/diff'
import DiffHunk from './DiffHunk'

export interface DiffFileCardProps {
  /** The file data */
  file: PatchFile
  /** Index of this file in the patch */
  fileIndex: number
  /** Whether the file content is expanded */
  isExpanded: boolean
  /** Toggle expand/collapse callback */
  onToggle: () => void
  /** Callback when a line is selected */
  onLineSelect?: (info: LineSelectInfo) => void
}

const DiffFileCard: React.FC<DiffFileCardProps> = ({
  file,
  fileIndex,
  isExpanded,
  onToggle,
  onLineSelect,
}) => {
  const [expandedHunks, setExpandedHunks] = useState<Record<number, boolean>>(() => {
    const initial: Record<number, boolean> = {}
    file.hunks.forEach((_, index) => {
      initial[index] = true
    })
    return initial
  })

  // Calculate file stats
  const fileStats = useMemo(() => {
    let additions = 0
    let deletions = 0

    for (const hunk of file.hunks) {
      for (const line of hunk.lines) {
        if (line.type === 'add') additions++
        if (line.type === 'remove') deletions++
      }
    }

    return { additions, deletions }
  }, [file])

  // Get display name
  const displayName = useMemo(() => {
    if (file.isNew) return file.newName || 'New file'
    if (file.isDeleted) return file.oldName || 'Deleted file'
    if (file.isRenamed) return `${file.oldName} → ${file.newName}`
    return file.newName || file.oldName || 'Unknown file'
  }, [file])

  // Get file status badge
  const getStatusBadge = () => {
    if (file.isNew) {
      return (
        <span style={{ ...styles.badge, ...styles.newBadge }}>New</span>
      )
    }
    if (file.isDeleted) {
      return (
        <span style={{ ...styles.badge, ...styles.deletedBadge }}>Deleted</span>
      )
    }
    if (file.isRenamed) {
      return (
        <span style={{ ...styles.badge, ...styles.renamedBadge }}>Renamed</span>
      )
    }
    if (file.isBinary) {
      return (
        <span style={{ ...styles.badge, ...styles.binaryBadge }}>Binary</span>
      )
    }
    return null
  }

  const toggleHunk = (hunkIndex: number) => {
    setExpandedHunks((prev) => ({
      ...prev,
      [hunkIndex]: !prev[hunkIndex],
    }))
  }

  return (
    <div className="diff-file-card" style={styles.container}>
      {/* File header */}
      <div
        className="diff-file-card__header"
        style={styles.header}
        onClick={onToggle}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggle()
          }
        }}
      >
        {/* Expand/collapse indicator */}
        <span
          style={{
            ...styles.chevron,
            transform: isExpanded ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        >
          <ChevronIcon />
        </span>

        {/* File icon */}
        <span style={styles.fileIcon}>
          <FileIcon />
        </span>

        {/* File name */}
        <span style={styles.fileName}>{displayName}</span>

        {/* Status badge */}
        {getStatusBadge()}

        {/* File stats */}
        <span style={styles.fileStats}>
          {fileStats.additions > 0 && (
            <span style={styles.additions}>+{fileStats.additions}</span>
          )}
          {fileStats.deletions > 0 && (
            <span style={styles.deletions}>-{fileStats.deletions}</span>
          )}
        </span>

        {/* Hunk count */}
        {file.hunks.length > 0 && (
          <span style={styles.hunkCount}>
            {file.hunks.length} {file.hunks.length === 1 ? 'hunk' : 'hunks'}
          </span>
        )}
      </div>

      {/* File content (hunks) */}
      {isExpanded && (
        <div className="diff-file-card__content" style={styles.content}>
          {file.isBinary ? (
            <div style={styles.binaryMessage}>Binary file not shown</div>
          ) : file.hunks.length === 0 ? (
            <div style={styles.emptyMessage}>No changes in this file</div>
          ) : (
            file.hunks.map((hunk, hunkIndex) => (
              <DiffHunk
                key={hunkIndex}
                hunk={hunk}
                fileIndex={fileIndex}
                hunkIndex={hunkIndex}
                isExpanded={expandedHunks[hunkIndex] ?? true}
                onToggle={() => toggleHunk(hunkIndex)}
                onLineSelect={onLineSelect}
              />
            ))
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const ChevronIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="9 18 15 12 9 6" />
  </svg>
)

const FileIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    borderBottom: '1px solid #21262d',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px',
    backgroundColor: '#161b22',
    cursor: 'pointer',
    userSelect: 'none',
  },
  chevron: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
    transition: 'transform 0.15s ease',
    flexShrink: 0,
  },
  fileIcon: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
    flexShrink: 0,
  },
  fileName: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#c9d1d9',
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  badge: {
    fontSize: '10px',
    fontWeight: 500,
    padding: '2px 6px',
    borderRadius: '4px',
    textTransform: 'uppercase',
  },
  newBadge: {
    backgroundColor: 'rgba(35, 134, 54, 0.2)',
    color: '#3fb950',
  },
  deletedBadge: {
    backgroundColor: 'rgba(248, 81, 73, 0.2)',
    color: '#f85149',
  },
  renamedBadge: {
    backgroundColor: 'rgba(88, 166, 255, 0.2)',
    color: '#58a6ff',
  },
  binaryBadge: {
    backgroundColor: 'rgba(139, 148, 158, 0.2)',
    color: '#8b949e',
  },
  fileStats: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  additions: {
    color: '#3fb950',
  },
  deletions: {
    color: '#f85149',
  },
  hunkCount: {
    fontSize: '11px',
    color: '#8b949e',
  },
  content: {
    backgroundColor: '#0d1117',
  },
  binaryMessage: {
    padding: '16px',
    textAlign: 'center',
    color: '#8b949e',
    fontSize: '12px',
    fontStyle: 'italic',
  },
  emptyMessage: {
    padding: '16px',
    textAlign: 'center',
    color: '#8b949e',
    fontSize: '12px',
    fontStyle: 'italic',
  },
}

export default DiffFileCard

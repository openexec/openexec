/**
 * DiffViewer Component
 * Displays unified diff patches with syntax highlighting for additions, deletions, and context.
 * @module components/chat/diff/DiffViewer
 */
import React, { useState, useMemo } from 'react'
import type { Patch, PatchStats, LineSelectInfo } from '../../../types/diff'
import DiffFileCard from './DiffFileCard'
import DiffStats from './DiffStats'
import { colors, typography, borderRadius } from '../../../utils/theme'
import { ExpandIcon, CollapseIcon } from '../../../utils/icons'

export interface DiffViewerProps {
  /** Parsed patch object */
  patch: Patch
  /** Patch statistics (optional - calculated if not provided) */
  stats?: PatchStats
  /** Whether files are expanded by default */
  defaultExpanded?: boolean
  /** Callback when a line is selected */
  onLineSelect?: (info: LineSelectInfo) => void
  /** Whether to show the stats header */
  showStats?: boolean
  /** Whether to show raw patch content fallback */
  showRawFallback?: boolean
}

const DiffViewer: React.FC<DiffViewerProps> = ({
  patch,
  stats: providedStats,
  defaultExpanded = true,
  onLineSelect,
  showStats = true,
  showRawFallback = true,
}) => {
  const [expandedFiles, setExpandedFiles] = useState<Record<number, boolean>>(() => {
    const initial: Record<number, boolean> = {}
    patch.files.forEach((_, index) => {
      initial[index] = defaultExpanded
    })
    return initial
  })

  // Calculate stats if not provided
  const stats = useMemo<PatchStats>(() => {
    if (providedStats) return providedStats

    let additions = 0
    let deletions = 0
    let hunks = 0

    for (const file of patch.files) {
      hunks += file.hunks.length
      for (const hunk of file.hunks) {
        for (const line of hunk.lines) {
          if (line.type === 'add') additions++
          if (line.type === 'remove') deletions++
        }
      }
    }

    return {
      filesChanged: patch.files.length,
      additions,
      deletions,
      hunks,
    }
  }, [patch, providedStats])

  const toggleFile = (index: number) => {
    setExpandedFiles((prev) => ({
      ...prev,
      [index]: !prev[index],
    }))
  }

  const expandAll = () => {
    const newState: Record<number, boolean> = {}
    patch.files.forEach((_, index) => {
      newState[index] = true
    })
    setExpandedFiles(newState)
  }

  const collapseAll = () => {
    const newState: Record<number, boolean> = {}
    patch.files.forEach((_, index) => {
      newState[index] = false
    })
    setExpandedFiles(newState)
  }

  // Check if all files are expanded
  const allExpanded = patch.files.every((_, index) => expandedFiles[index])

  if (patch.files.length === 0 && showRawFallback && patch.rawContent) {
    return (
      <div style={styles.container}>
        <div style={styles.header}>
          <span style={styles.title}>Patch</span>
        </div>
        <pre style={styles.rawContent}>
          <code>{patch.rawContent}</code>
        </pre>
      </div>
    )
  }

  return (
    <div className="diff-viewer" style={styles.container}>
      {/* Header with stats and controls */}
      <div style={styles.header}>
        <span style={styles.title}>Changes</span>
        {showStats && <DiffStats stats={stats} compact />}
        <div style={styles.headerControls}>
          <button
            onClick={allExpanded ? collapseAll : expandAll}
            style={styles.toggleAllButton}
            title={allExpanded ? 'Collapse all files' : 'Expand all files'}
          >
            {allExpanded ? (
              <>
                <CollapseIcon />
                <span>Collapse all</span>
              </>
            ) : (
              <>
                <ExpandIcon />
                <span>Expand all</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* File list */}
      <div style={styles.fileList}>
        {patch.files.map((file, fileIndex) => (
          <DiffFileCard
            key={fileIndex}
            file={file}
            fileIndex={fileIndex}
            isExpanded={expandedFiles[fileIndex] ?? defaultExpanded}
            onToggle={() => toggleFile(fileIndex)}
            onLineSelect={onLineSelect}
          />
        ))}
      </div>
    </div>
  )
}

// Styles - using centralized theme
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: colors.bg.primary,
    borderRadius: borderRadius.xl,
    border: `1px solid ${colors.bg.border}`,
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    padding: '10px 12px',
    backgroundColor: colors.bg.secondary,
    borderBottom: `1px solid ${colors.bg.border}`,
  },
  title: {
    fontSize: typography.fontSize.md,
    fontWeight: 600,
    color: colors.text.primary,
  },
  headerControls: {
    marginLeft: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  toggleAllButton: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    padding: '4px 8px',
    fontSize: typography.fontSize.sm,
    color: colors.text.secondary,
    backgroundColor: 'transparent',
    border: `1px solid ${colors.bg.border}`,
    borderRadius: borderRadius.md,
    cursor: 'pointer',
  },
  fileList: {
    display: 'flex',
    flexDirection: 'column',
  },
  rawContent: {
    margin: 0,
    padding: '12px',
    fontSize: typography.fontSize.base,
    fontFamily: typography.fontFamily.mono,
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
    color: colors.text.primary,
    backgroundColor: colors.bg.primary,
  },
}

export default DiffViewer

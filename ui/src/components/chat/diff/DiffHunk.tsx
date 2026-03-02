/**
 * DiffHunk Component
 * Displays a single hunk with line numbers and diff highlighting.
 * @module components/chat/diff/DiffHunk
 */
import React, { useMemo } from 'react'
import type { PatchHunk, PatchLine, LineSelectInfo } from '../../../types/diff'

export interface DiffHunkProps {
  /** The hunk data */
  hunk: PatchHunk
  /** File index for selection callbacks */
  fileIndex: number
  /** Hunk index for selection callbacks */
  hunkIndex: number
  /** Whether the hunk content is expanded */
  isExpanded: boolean
  /** Toggle expand/collapse callback */
  onToggle: () => void
  /** Callback when a line is selected */
  onLineSelect?: (info: LineSelectInfo) => void
}

interface LineWithNumbers {
  line: PatchLine
  oldLineNumber: number | null
  newLineNumber: number | null
}

const DiffHunk: React.FC<DiffHunkProps> = ({
  hunk,
  fileIndex,
  hunkIndex,
  isExpanded,
  onToggle,
  onLineSelect,
}) => {
  // Calculate line numbers for each line
  const linesWithNumbers = useMemo<LineWithNumbers[]>(() => {
    let oldLine = hunk.oldStart
    let newLine = hunk.newStart

    return hunk.lines.map((line) => {
      let oldLineNumber: number | null = null
      let newLineNumber: number | null = null

      switch (line.type) {
        case 'context':
          oldLineNumber = oldLine++
          newLineNumber = newLine++
          break
        case 'remove':
          oldLineNumber = oldLine++
          break
        case 'add':
          newLineNumber = newLine++
          break
      }

      return { line, oldLineNumber, newLineNumber }
    })
  }, [hunk])

  const handleLineClick = (lineData: LineWithNumbers, lineIndex: number) => {
    if (onLineSelect) {
      onLineSelect({
        fileIndex,
        hunkIndex,
        lineIndex,
        line: lineData.line,
        oldLineNumber: lineData.oldLineNumber,
        newLineNumber: lineData.newLineNumber,
      })
    }
  }

  // Get context from hunk header (function name, etc.)
  const headerContext = useMemo(() => {
    // Extract context after the @@ markers
    const match = hunk.header.match(/@@ .+? @@\s*(.*)$/)
    return match?.[1] || ''
  }, [hunk.header])

  return (
    <div className="diff-hunk" style={styles.container}>
      {/* Hunk header */}
      <div
        className="diff-hunk__header"
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
        <span style={styles.headerRange}>
          @@ -{hunk.oldStart},{hunk.oldCount} +{hunk.newStart},{hunk.newCount} @@
        </span>
        {headerContext && (
          <span style={styles.headerContext}>{headerContext}</span>
        )}
        <span
          style={{
            ...styles.chevron,
            transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
          }}
        >
          <ChevronIcon />
        </span>
      </div>

      {/* Hunk lines */}
      {isExpanded && (
        <div className="diff-hunk__lines" style={styles.lines}>
          <table style={styles.table}>
            <tbody>
              {linesWithNumbers.map((lineData, lineIndex) => (
                <DiffLine
                  key={lineIndex}
                  lineData={lineData}
                  onClick={() => handleLineClick(lineData, lineIndex)}
                  isSelectable={!!onLineSelect}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// DiffLine subcomponent for individual line rendering
interface DiffLineProps {
  lineData: LineWithNumbers
  onClick: () => void
  isSelectable: boolean
}

const DiffLine: React.FC<DiffLineProps> = ({ lineData, onClick, isSelectable }) => {
  const { line, oldLineNumber, newLineNumber } = lineData

  const getLineStyle = (): React.CSSProperties => {
    switch (line.type) {
      case 'add':
        return styles.lineAdd
      case 'remove':
        return styles.lineRemove
      default:
        return styles.lineContext
    }
  }

  const getPrefix = (): string => {
    switch (line.type) {
      case 'add':
        return '+'
      case 'remove':
        return '-'
      default:
        return ' '
    }
  }

  return (
    <tr
      className={`diff-line diff-line--${line.type}`}
      style={{
        ...styles.line,
        ...getLineStyle(),
        cursor: isSelectable ? 'pointer' : 'default',
      }}
      onClick={isSelectable ? onClick : undefined}
    >
      {/* Old line number */}
      <td style={styles.lineNumber}>
        {oldLineNumber !== null ? oldLineNumber : ''}
      </td>
      {/* New line number */}
      <td style={styles.lineNumber}>
        {newLineNumber !== null ? newLineNumber : ''}
      </td>
      {/* Line prefix */}
      <td style={styles.linePrefix}>{getPrefix()}</td>
      {/* Line content */}
      <td style={styles.lineContent}>
        <pre style={styles.lineContentPre}>{line.content}</pre>
      </td>
    </tr>
  )
}

// Icon components
const ChevronIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="6 9 12 15 18 9" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    borderTop: '1px solid #21262d',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '6px 12px',
    backgroundColor: 'rgba(56, 139, 253, 0.1)',
    cursor: 'pointer',
    userSelect: 'none',
  },
  headerRange: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#79c0ff',
  },
  headerContext: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#8b949e',
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  chevron: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
    transition: 'transform 0.15s ease',
    flexShrink: 0,
  },
  lines: {
    overflow: 'auto',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse',
    tableLayout: 'fixed',
  },
  line: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  lineContext: {
    backgroundColor: '#0d1117',
    color: '#c9d1d9',
  },
  lineAdd: {
    backgroundColor: 'rgba(35, 134, 54, 0.15)',
    color: '#3fb950',
  },
  lineRemove: {
    backgroundColor: 'rgba(218, 54, 51, 0.15)',
    color: '#f85149',
  },
  lineNumber: {
    width: '40px',
    minWidth: '40px',
    maxWidth: '40px',
    padding: '0 8px',
    textAlign: 'right',
    color: '#484f58',
    backgroundColor: 'rgba(0, 0, 0, 0.2)',
    userSelect: 'none',
    verticalAlign: 'top',
    fontSize: '11px',
    lineHeight: '20px',
  },
  linePrefix: {
    width: '16px',
    minWidth: '16px',
    maxWidth: '16px',
    padding: '0 4px',
    textAlign: 'center',
    userSelect: 'none',
    verticalAlign: 'top',
    lineHeight: '20px',
  },
  lineContent: {
    padding: '0 8px',
    verticalAlign: 'top',
    lineHeight: '20px',
  },
  lineContentPre: {
    margin: 0,
    padding: 0,
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-all',
    lineHeight: '20px',
  },
}

export default DiffHunk

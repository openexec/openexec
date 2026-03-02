/**
 * ToolCallOutput Component
 * Tool execution result display.
 * @module components/chat/tools/ToolCallOutput
 */
import React, { useState, useMemo } from 'react'

export interface ToolCallOutputProps {
  output?: string
  isError?: boolean
  duration?: number
}

const ToolCallOutput: React.FC<ToolCallOutputProps> = ({ output, isError = false, duration }) => {
  const [isExpanded, setIsExpanded] = useState(false)

  // Try to format JSON output
  const formattedOutput = useMemo(() => {
    if (!output) return ''
    try {
      const parsed = JSON.parse(output)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return output
    }
  }, [output])

  // Determine if output is long enough to collapse
  const shouldCollapse = formattedOutput.length > 300 || formattedOutput.split('\n').length > 8

  if (!output) {
    return null
  }

  return (
    <div
      className={`tool-call-output ${isError ? 'tool-call-output--error' : ''}`}
      style={{
        ...styles.container,
        borderColor: isError ? '#f85149' : '#30363d',
      }}
    >
      <div style={styles.header}>
        <span style={{ ...styles.label, color: isError ? '#f85149' : '#8b949e' }}>
          {isError ? 'Error' : 'Output'}
        </span>
        <div style={styles.meta}>
          {duration !== undefined && (
            <span style={styles.duration}>{duration}ms</span>
          )}
          {shouldCollapse && (
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              style={styles.toggleButton}
            >
              {isExpanded ? 'Collapse' : 'Expand'}
            </button>
          )}
        </div>
      </div>
      <pre
        style={{
          ...styles.code,
          maxHeight: !shouldCollapse || isExpanded ? 'none' : '150px',
          overflow: 'auto',
          color: isError ? '#f85149' : '#c9d1d9',
        }}
      >
        <code>{formattedOutput}</code>
      </pre>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#0d1117',
    borderRadius: '6px',
    border: '1px solid #30363d',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '8px 12px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
  },
  label: {
    fontSize: '12px',
    fontWeight: 500,
  },
  meta: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  duration: {
    fontSize: '11px',
    color: '#8b949e',
  },
  toggleButton: {
    fontSize: '11px',
    color: '#58a6ff',
    backgroundColor: 'transparent',
    border: 'none',
    cursor: 'pointer',
    padding: '2px 6px',
  },
  code: {
    margin: 0,
    padding: '12px',
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
}

export default ToolCallOutput

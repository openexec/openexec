/**
 * ToolCallInput Component
 * Collapsible JSON input viewer.
 * @module components/chat/tools/ToolCallInput
 */
import React, { useState, useMemo } from 'react'

export interface ToolCallInputProps {
  input: string
  toolName: string
}

const ToolCallInput: React.FC<ToolCallInputProps> = ({ input, toolName }) => {
  const [isExpanded, setIsExpanded] = useState(false)

  // Try to parse and format JSON
  const formattedInput = useMemo(() => {
    try {
      const parsed = JSON.parse(input)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return input
    }
  }, [input])

  // Determine if input is long enough to collapse
  const shouldCollapse = formattedInput.length > 200 || formattedInput.split('\n').length > 5

  return (
    <div className="tool-call-input" style={styles.container}>
      <div style={styles.header}>
        <span style={styles.label}>Input for {toolName}</span>
        {shouldCollapse && (
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            style={styles.toggleButton}
          >
            {isExpanded ? 'Collapse' : 'Expand'}
          </button>
        )}
      </div>
      <pre
        style={{
          ...styles.code,
          maxHeight: !shouldCollapse || isExpanded ? 'none' : '100px',
          overflow: 'auto',
        }}
      >
        <code>{formattedInput}</code>
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
    color: '#8b949e',
    fontWeight: 500,
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
    color: '#c9d1d9',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
}

export default ToolCallInput

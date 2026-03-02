/**
 * ToolCallList Component
 * Container for tool calls in a message.
 * @module components/chat/tools/ToolCallList
 */
import React from 'react'
import type { ToolCall } from '../../../types/chat'
import ToolCallCard from './ToolCallCard'

export interface ToolCallListProps {
  toolCalls: ToolCall[]
  onApprove?: (toolCallId: string) => void
  onReject?: (toolCallId: string, reason?: string) => void
}

const ToolCallList: React.FC<ToolCallListProps> = ({ toolCalls, onApprove, onReject }) => {
  if (toolCalls.length === 0) {
    return null
  }

  return (
    <div className="tool-call-list" style={styles.container}>
      <div style={styles.header}>
        <span style={styles.title}>Tool Calls ({toolCalls.length})</span>
      </div>
      <div style={styles.list}>
        {toolCalls.map((tc) => (
          <ToolCallCard
            key={tc.id}
            toolCall={tc}
            onApprove={onApprove ? () => onApprove(tc.id) : undefined}
            onReject={onReject ? (reason) => onReject(tc.id, reason) : undefined}
          />
        ))}
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  title: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  list: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
}

export default ToolCallList
